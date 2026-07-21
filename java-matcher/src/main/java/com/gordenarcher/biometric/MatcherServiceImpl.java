package com.gordenarcher.biometric;

import com.gordenarcher.biometric.grpc.*;
import com.google.protobuf.ByteString;
import com.machinezoo.sourceafis.FingerprintImage;
import com.machinezoo.sourceafis.FingerprintImageOptions;
import com.machinezoo.sourceafis.FingerprintMatcher;
import com.machinezoo.sourceafis.FingerprintTemplate;
import io.grpc.Status;
import io.grpc.stub.StreamObserver;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;

/**
 * Deliberately holds no state and no dependency on a datastore. Go decides
 * what gets persisted and how it's encrypted, this class only ever sees
 * bytes it was handed in the current request and returns bytes/scores,
 * nothing here should ever need to survive past a single RPC call.
 */
public final class MatcherServiceImpl extends MatcherGrpc.MatcherImplBase {

    // Confirmed against SourceAFIS's own docs rather than guessed: their
    // scale isn't 0-1, it's open ended, and they publish threshold 10 as
    // roughly 10% false match rate, 20 as 1%, 30 as 0.1%. 40 is comfortably
    // stricter than that, a reasonable starting point for something that
    // gates a government ID reprint, but this should still be revisited
    // once there's real enrollment data to test false accept/reject rates
    // against, not treated as a tuned production value.
    private static final double DEFAULT_MATCH_THRESHOLD = 40.0;

    // SourceAFIS documents 500 as the DPI its feature extractor is tuned
    // against, this must match whatever the real sensor SDK actually
    // outputs, a wrong DPI here silently degrades match quality rather
    // than throwing an obvious error, worth double checking against
    // whatever hardware this ends up wired to.
    private static final int SCANNER_DPI = 500;

    @Override
    public void enroll(EnrollRequest request, StreamObserver<EnrollResponse> responseObserver) {
        try {
            byte[] scanBytes = request.getScanData().toByteArray();

            FingerprintImage image = new FingerprintImage(
                    scanBytes,
                    new FingerprintImageOptions().dpi(SCANNER_DPI));

            // This is the expensive feature extraction step, SourceAFIS's
            // own docs call this constructor out specifically as costly,
            // it is why the serialized template gets cached by Go rather
            // than re-derived from the raw scan on every verify.
            FingerprintTemplate template = new FingerprintTemplate(image);
            byte[] templateBytes = template.toByteArray();

            EnrollResponse response = EnrollResponse.newBuilder()
                    .setTemplate(ByteString.copyFrom(templateBytes))
                    // SourceAFIS does not expose a documented standalone
                    // quality score the way NFIQ2 does, so this is left
                    // unset (0) rather than fabricating a number. If scan
                    // quality gating turns out to matter in practice, an
                    // NFIQ2 pass ahead of enrollment is the real fix, not
                    // something to approximate here.
                    .setQualityScore(0f)
                    .build();

            responseObserver.onNext(response);
            responseObserver.onCompleted();
        } catch (Exception e) {
            // Caught broadly and surfaced as INTERNAL rather than letting
            // the exception escape, a raw exception here would kill the
            // stream without a status code the Go client can branch on.
            responseObserver.onError(
                    Status.INTERNAL.withDescription("enrollment failed").withCause(e).asRuntimeException());
        }
    }

    @Override
    public void verify(VerifyRequest request, StreamObserver<VerifyResponse> responseObserver) {
        try {
            byte[] probeBytes = request.getProbeScan().toByteArray();
            byte[] candidateBytes = request.getCandidateTemplate().toByteArray();

            double score = scoreAgainst(probeBytes, candidateBytes);

            VerifyResponse response = VerifyResponse.newBuilder()
                    .setMatch(score >= DEFAULT_MATCH_THRESHOLD)
                    .setScore(score)
                    .build();

            responseObserver.onNext(response);
            responseObserver.onCompleted();
        } catch (Exception e) {
            responseObserver.onError(
                    Status.INTERNAL.withDescription("verification failed").withCause(e).asRuntimeException());
        }
    }

    @Override
    public void identify(IdentifyRequest request, StreamObserver<IdentifyResponse> responseObserver) {
        try {
            byte[] probeBytes = request.getProbeScan().toByteArray();

            FingerprintImage probeImage = new FingerprintImage(
                    probeBytes,
                    new FingerprintImageOptions().dpi(SCANNER_DPI));
            FingerprintTemplate probeTemplate = new FingerprintTemplate(probeImage);

            // Built once per request, then reused across every candidate.
            // SourceAFIS's docs are explicit that the FingerprintMatcher
            // constructor is the expensive part, precisely because it's
            // built to be reused for many match() calls against one
            // probe, rebuilding it per candidate would throw away the
            // entire reason this class exists.
            FingerprintMatcher matcher = new FingerprintMatcher(probeTemplate);

            List<IdentifyMatch> matches = new ArrayList<>();
            // Still a linear scan over whatever batch Go sent, that part
            // of the scaling limit is real, real 1:N scale at population
            // size needs binning/indexing on the candidate side (grouping
            // by pattern type before this point), not something to solve
            // inside this loop.
            for (TemplateCandidate candidate : request.getCandidatesList()) {
                FingerprintTemplate candidateTemplate =
                        new FingerprintTemplate(candidate.getTemplate().toByteArray());
                double score = matcher.match(candidateTemplate);
                if (score >= DEFAULT_MATCH_THRESHOLD) {
                    matches.add(IdentifyMatch.newBuilder()
                            .setCandidateId(candidate.getCandidateId())
                            .setScore(score)
                            .build());
                }
            }

            // Highest score first, callers care about the best match, not
            // insertion order.
            matches.sort(Comparator.comparingDouble(IdentifyMatch::getScore).reversed());

            IdentifyResponse response = IdentifyResponse.newBuilder()
                    .addAllMatches(matches)
                    .build();

            responseObserver.onNext(response);
            responseObserver.onCompleted();
        } catch (Exception e) {
            responseObserver.onError(
                    Status.INTERNAL.withDescription("identification failed").withCause(e).asRuntimeException());
        }
    }

    private double scoreAgainst(byte[] probeBytes, byte[] candidateTemplateBytes) {
        FingerprintImage probeImage = new FingerprintImage(
                probeBytes,
                new FingerprintImageOptions().dpi(SCANNER_DPI));
        FingerprintTemplate probe = new FingerprintTemplate(probeImage);

        // The byte[] constructor deserializes the CBOR-encoded template
        // Enroll() produced, this is what makes storing Enroll's output
        // and feeding it back in later actually work, rather than needing
        // the original raw scan again.
        FingerprintTemplate candidate = new FingerprintTemplate(candidateTemplateBytes);

        // Fine to build a fresh FingerprintMatcher per call here since
        // Verify only ever compares against one candidate. Identify has
        // its own matcher construction, built once per probe instead of
        // once per candidate, since that path actually needs the reuse.
        return new FingerprintMatcher(probe).match(candidate);
    }
}