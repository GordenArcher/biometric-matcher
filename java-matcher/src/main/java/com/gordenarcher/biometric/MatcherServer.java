package com.gordenarcher.biometric;

import io.grpc.Server;
// grpc-netty-shaded relocates everything under this package, including
// NettyServerBuilder and GrpcSslContexts themselves, not just the
// underlying Netty classes they hand back. Confirmed against grpc-java's
// own ShadingTest.java, which imports from exactly this path, my first
// pass at this only relocated the netty.handler.ssl.* imports and left
// NettyServerBuilder/GrpcSslContexts unshaded, which doesn't compile.
import io.grpc.netty.shaded.io.grpc.netty.GrpcSslContexts;
import io.grpc.netty.shaded.io.grpc.netty.NettyServerBuilder;
import io.grpc.netty.shaded.io.netty.handler.ssl.ClientAuth;
import io.grpc.netty.shaded.io.netty.handler.ssl.SslContext;

import java.io.File;
import java.io.IOException;

public final class MatcherServer {

    private static final int DEFAULT_PORT = 50051;

    public static void main(String[] args) throws IOException, InterruptedException {
        int port = resolvePort();

        SslContext sslContext = buildSslContext();

        Server server = NettyServerBuilder.forPort(port)
                .sslContext(sslContext)
                .addService(new MatcherServiceImpl())
                .build();

        server.start();
        System.out.println("biometric-matcher listening on " + port + " (mTLS required)");

        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("shutting down biometric-matcher");
            server.shutdown();
        }));

        server.awaitTermination();
    }

    // TLS is not optional here, there is no plaintext fallback path.
    // This service holds fingerprint templates in transit, requiring
    // three specific env vars to be set and failing to start otherwise
    // is deliberate, a silent fallback to plaintext would be the kind
    // of thing that's fine in dev and forgotten in production.
    private static SslContext buildSslContext() throws IOException {
        File certChain = requireFile("MATCHER_TLS_CERT");
        File privateKey = requireFile("MATCHER_TLS_KEY");
        File clientCa = requireFile("MATCHER_TLS_CLIENT_CA");

        return GrpcSslContexts.forServer(certChain, privateKey)
                // REQUIRE, not OPTIONAL, an unauthenticated go-client
                // should not be able to reach Enroll/Verify/Identify at
                // all, not just be treated as untrusted once connected.
                .clientAuth(ClientAuth.REQUIRE)
                .trustManager(clientCa)
                .build();
    }

    private static File requireFile(String envVar) {
        String path = System.getenv(envVar);
        if (path == null || path.isBlank()) {
            throw new IllegalStateException(
                    "environment variable " + envVar + " is required (mTLS is not optional for this service)");
        }
        File file = new File(path);
        if (!file.canRead()) {
            throw new IllegalStateException(envVar + " points to " + path + ", which is not readable");
        }
        return file;
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