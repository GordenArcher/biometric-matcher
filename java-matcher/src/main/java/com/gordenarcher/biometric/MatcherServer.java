package com.gordenarcher.biometric;

import io.grpc.Server;
import io.grpc.ServerBuilder;

import java.io.IOException;

public final class MatcherServer {

    // Kept as a constant rather than reading env directly here, once this
    // needs more than one config value it should move to a proper config
    // loader instead of scattered System.getenv calls.
    private static final int DEFAULT_PORT = 50051;

    public static void main(String[] args) throws IOException, InterruptedException {
        int port = resolvePort();

        Server server = ServerBuilder.forPort(port)
                .addService(new MatcherServiceImpl())
                .build();

        server.start();
        System.out.println("biometric-matcher listening on " + port);

        // Blocks the main thread so the JVM doesn't exit immediately.
        // awaitTermination is the standard grpc-java pattern for a long
        // running server process, not a polling loop.
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("shutting down biometric-matcher");
            server.shutdown();
        }));

        server.awaitTermination();
    }

    private static int resolvePort() {
        String raw = System.getenv("MATCHER_PORT");
        if (raw == null || raw.isBlank()) {
            return DEFAULT_PORT;
        }
        return Integer.parseInt(raw);
    }

    private MatcherServer() {
    }
}
