package com.gordenarcher.biometric;

import com.gordenarcher.biometric.grpc.EnrollRequest;
import com.gordenarcher.biometric.grpc.EnrollResponse;
import com.gordenarcher.biometric.grpc.IdentifyRequest;
import com.gordenarcher.biometric.grpc.IdentifyResponse;
import com.gordenarcher.biometric.grpc.ScanFormat;
import com.gordenarcher.biometric.grpc.TemplateCandidate;
import com.gordenarcher.biometric.grpc.VerifyRequest;
import com.gordenarcher.biometric.grpc.VerifyResponse;
import com.google.protobuf.ByteString;
import io.grpc.stub.StreamObserver;
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Runs against real FVC2002 DB1_B scans rather than synthetic bytes.
 * SourceAFIS's scoring depends on real minutiae geometry, a fabricated
 * byte array would either fail extraction outright or produce a score
 * with no real meaning, testing against the same public dataset the
 * README's "Verified working" section uses keeps this test honest about
 * what it's actually checking.
 *
 * Skips (not fails) if scripts/fetch-testdata.sh hasn't been run, this
 * lets `gradle test` succeed in a checkout that hasn't pulled the
 * dataset yet, rather than every fresh clone starting from a red test.
 */
class MatcherServiceImplTest {

    // Relative to the java-matcher module root, gradle's default test
    // working directory, matching how the CLI's own testdata/README.md
    // documents these paths relative to go-client instead confirms this
    // needs its own relative step up.
    private static final Path TESTDATA_DIR = Paths.get("../testdata/fvc2002-db1");

    private MatcherServiceImpl service;

    @BeforeEach
    void setUp() {
        Assumptions.assumeTrue(
                Files.isDirectory(TESTDATA_DIR),
                "testdata not found at " + TESTDATA_DIR.toAbsolutePath()
                        + ", run scripts/fetch-testdata.sh from the repo root first");
        service = new MatcherServiceImpl();
    }

    @Test
    void verifySameFingerDifferentImpressionMatches() throws IOException {
        byte[] template = enroll("101_1.tif");
        VerifyResponse resp = verify("101_2.tif", template);

        assertTrue(resp.getMatch(), "expected same finger, different impression, to match");
        assertTrue(resp.getScore() > 0, "expected a positive score for a genuine match");
    }

    @Test
    void verifyDifferentFingerDoesNotMatch() throws IOException {
        byte[] template = enroll("101_1.tif");
        VerifyResponse resp = verify("102_1.tif", template);

        assertFalse(resp.getMatch(), "expected two different fingers not to match");
    }

    @Test
    void identifyFindsTheCorrectCandidateAmongSeveral() throws IOException {
        byte[] template101 = enroll("101_1.tif");
        byte[] template102 = enroll("102_1.tif");
        byte[] template103 = enroll("103_1.tif");

        IdentifyRequest request = IdentifyRequest.newBuilder()
                .setProbeScan(ByteString.copyFrom(readScan("101_2.tif")))
                .setFormat(ScanFormat.SCAN_FORMAT_ISO_19794_2)
                .addCandidates(candidate("person-101", template101))
                .addCandidates(candidate("person-102", template102))
                .addCandidates(candidate("person-103", template103))
                .build();

        CapturingObserver<IdentifyResponse> observer = new CapturingObserver<>();
        service.identify(request, observer);

        assertEquals(1, observer.value.getMatchesCount(),
                "expected exactly one candidate to clear the match threshold");
        assertEquals("person-101", observer.value.getMatches(0).getCandidateId());
    }

    private byte[] enroll(String filename) throws IOException {
        EnrollRequest request = EnrollRequest.newBuilder()
                .setScanData(ByteString.copyFrom(readScan(filename)))
                .setFormat(ScanFormat.SCAN_FORMAT_ISO_19794_2)
                .build();

        CapturingObserver<EnrollResponse> observer = new CapturingObserver<>();
        service.enroll(request, observer);
        return observer.value.getTemplate().toByteArray();
    }

    private VerifyResponse verify(String probeFilename, byte[] candidateTemplate) throws IOException {
        VerifyRequest request = VerifyRequest.newBuilder()
                .setProbeScan(ByteString.copyFrom(readScan(probeFilename)))
                .setFormat(ScanFormat.SCAN_FORMAT_ISO_19794_2)
                .setCandidateTemplate(ByteString.copyFrom(candidateTemplate))
                .build();

        CapturingObserver<VerifyResponse> observer = new CapturingObserver<>();
        service.verify(request, observer);
        return observer.value;
    }

    private TemplateCandidate candidate(String id, byte[] template) {
        return TemplateCandidate.newBuilder()
                .setCandidateId(id)
                .setTemplate(ByteString.copyFrom(template))
                .build();
    }

    private byte[] readScan(String filename) throws IOException {
        return Files.readAllBytes(TESTDATA_DIR.resolve(filename));
    }

    /**
     * MatcherServiceImpl's methods run synchronously to completion in
     * the current thread, no async work happens inside them, a plain
     * field capture is enough here, this is not a real streaming or
     * concurrent-call scenario that would need synchronization.
     */
    private static final class CapturingObserver<T> implements StreamObserver<T> {
        T value;
        Throwable error;

        @Override
        public void onNext(T v) {
            value = v;
        }

        @Override
        public void onError(Throwable t) {
            error = t;
        }

        @Override
        public void onCompleted() {
            // Nothing to do, value/error already captured by the calls
            // above, this method existing is just satisfying the
            // interface.
        }
    }
}