package com.gordenarcher.biometric;

import com.gordenarcher.biometric.grpc.*;
import com.google.protobuf.ByteString;
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
 *
 * TODO: the exact SourceAFIS 3.x API calls below (FingerprintImage,
 * FingerprintTemplate, FingerprintMatcher) are written from general
 * knowledge of the library's shape, not verified against the current
 * released version. Confirm constructor signatures and the matcher's
 * score scale against the SourceAFIS docs before relying on this.
 */
public final class MatcherServiceImpl extends MatcherGrpc.MatcherImplBase {

    // SourceAFIS scores are open ended, not 0-1 normalized. This threshold
    // is a starting point, not a tuned value, revisit once there's real
    // enrollment data to test false accept/reject rates against.
    private static final double DEFAULT_MATCH_THRESHOLD = 40.0;

    @Override
    public void enroll(EnrollRequest request, StreamObserver<EnrollResponse> responseObserver) {
        try {
            byte[] scanBytes = request.getScanData().toByteArray();

            // TODO: swap for the real SourceAFIS extraction call, this is
            // a placeholder shape (image -> template -> serialized bytes).
            // com.machinezoo.sourceafis.FingerprintTemplate template =
            //     new com.machinezoo.sourceafis.FingerprintTemplate(
            //         new com.machinezoo.sourceafis.FingerprintImage(scanBytes));
            // byte[] templateBytes = template.toByteArray();
            byte[] templateBytes = scanBytes; // placeholder until wired up

            EnrollResponse response = EnrollResponse.newBuilder()
                    .setTemplate(ByteString.copyFrom(templateBytes))
                    // TODO: pull the real quality score SourceAFIS reports
                    // during extraction instead of hardcoding this.
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

            List<IdentifyMatch> matches = new ArrayList<>();
            // Linear scan over whatever batch Go sent. Fine for a demo,
            // real scale needs SourceAFIS's own indexing/matcher pooling
            // rather than scoring candidates one at a time in a loop, note
            // this in the README rather than pretending this scales as is.
            for (TemplateCandidate candidate : request.getCandidatesList()) {
                double score = scoreAgainst(probeBytes, candidate.getTemplate().toByteArray());
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
        // TODO: real implementation, roughly:
        // FingerprintTemplate probe =
        //     new FingerprintTemplate(new FingerprintImage(probeBytes));
        // FingerprintTemplate candidate =
        //     FingerprintTemplate.deserialize(candidateTemplateBytes);
        // return new FingerprintMatcher(probe).match(candidate);
        throw new UnsupportedOperationException(
                "wire up SourceAFIS matching before calling this, see TODO above");
    }
}
