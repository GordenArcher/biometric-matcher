package com.gordenarcher.biometric;

import javax.net.ssl.SSLEngine;
import javax.net.ssl.TrustManagerFactory;
import javax.net.ssl.X509ExtendedTrustManager;
import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.net.Socket;
import java.nio.file.Files;
import java.security.GeneralSecurityException;
import java.security.KeyStore;
import java.security.cert.Certificate;
import java.security.cert.CertificateException;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * Delegates ordinary chain-of-trust verification to the JDK's own
 * X509ExtendedTrustManager built from our CA cert, then adds one extra
 * check: is the leaf cert's serial in the revoked list. This is
 * deliberately a flat file, not a real CRL/OCSP implementation, see the
 * comment in scripts/revoke-cert.sh for why that's the right amount of
 * machinery here.
 *
 * The revoked list is re-read from disk on every check rather than
 * cached at construction time, so revoking a cert takes effect on the
 * next handshake without restarting this process, at the cost of a small
 * file read per connection, a fine trade for a list this size.
 */
final class RevocationCheckingTrustManager extends X509ExtendedTrustManager {

    private final X509ExtendedTrustManager delegate;
    private final File revokedSerialsFile;

    static RevocationCheckingTrustManager create(File caCertFile, File revokedSerialsFile) throws GeneralSecurityException, IOException {
        KeyStore keyStore = KeyStore.getInstance(KeyStore.getDefaultType());
        keyStore.load(null, null);

        CertificateFactory certFactory = CertificateFactory.getInstance("X.509");
        try (FileInputStream in = new FileInputStream(caCertFile)) {
            Certificate ca = certFactory.generateCertificate(in);
            keyStore.setCertificateEntry("ca", ca);
        }

        TrustManagerFactory factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        factory.init(keyStore);

        for (var tm : factory.getTrustManagers()) {
            if (tm instanceof X509ExtendedTrustManager extended) {
                return new RevocationCheckingTrustManager(extended, revokedSerialsFile);
            }
        }

        // Shouldn't happen with the default algorithm on a standard JDK,
        // failing loudly here beats silently falling back to a trust
        // manager that doesn't actually check revocation.
        throw new IllegalStateException("no X509ExtendedTrustManager found for " + TrustManagerFactory.getDefaultAlgorithm());
    }

    private RevocationCheckingTrustManager(X509ExtendedTrustManager delegate, File revokedSerialsFile) {
        this.delegate = delegate;
        this.revokedSerialsFile = revokedSerialsFile;
    }

    @Override
    public void checkClientTrusted(X509Certificate[] chain, String authType, Socket socket) throws CertificateException {
        delegate.checkClientTrusted(chain, authType, socket);
        checkNotRevoked(chain);
    }

    @Override
    public void checkClientTrusted(X509Certificate[] chain, String authType, SSLEngine engine) throws CertificateException {
        delegate.checkClientTrusted(chain, authType, engine);
        checkNotRevoked(chain);
    }

    @Override
    public void checkClientTrusted(X509Certificate[] chain, String authType) throws CertificateException {
        delegate.checkClientTrusted(chain, authType);
        checkNotRevoked(chain);
    }

    @Override
    public void checkServerTrusted(X509Certificate[] chain, String authType, Socket socket) throws CertificateException {
        delegate.checkServerTrusted(chain, authType, socket);
        checkNotRevoked(chain);
    }

    @Override
    public void checkServerTrusted(X509Certificate[] chain, String authType, SSLEngine engine) throws CertificateException {
        delegate.checkServerTrusted(chain, authType, engine);
        checkNotRevoked(chain);
    }

    @Override
    public void checkServerTrusted(X509Certificate[] chain, String authType) throws CertificateException {
        delegate.checkServerTrusted(chain, authType);
        checkNotRevoked(chain);
    }

    @Override
    public X509Certificate[] getAcceptedIssuers() {
        return delegate.getAcceptedIssuers();
    }

    private void checkNotRevoked(X509Certificate[] chain) throws CertificateException {
        if (chain.length == 0) {
            return;
        }

        // Leaf only, index 0 is always the end-entity cert per the
        // javax.net.ssl.X509TrustManager contract, chain[1..] are
        // intermediates/CA, which this project doesn't revoke, only
        // individual server/client certs.
        String serial = chain[0].getSerialNumber().toString(16).toUpperCase();

        Set<String> revoked;
        try {
            revoked = loadRevokedSerials();
        } catch (IOException e) {
            // Fail closed, not open, if the revoked-list file can't be
            // read for some reason (permissions, disk issue), refusing
            // the connection is the safe failure mode, silently treating
            // "can't check" as "not revoked" would defeat the point of
            // having this at all.
            throw new CertificateException("could not read revoked serials file, refusing connection", e);
        }

        if (revoked.contains(serial)) {
            throw new CertificateException("certificate serial " + serial + " has been revoked");
        }
    }

    private Set<String> loadRevokedSerials() throws IOException {
        if (!revokedSerialsFile.exists()) {
            // Missing file is a real error, not "nothing revoked", see
            // scripts/gen-certs.sh, which creates it empty on purpose so
            // this distinction is meaningful.
            throw new IOException("revoked serials file does not exist: " + revokedSerialsFile);
        }

        List<String> lines = Files.readAllLines(revokedSerialsFile.toPath());
        Set<String> serials = new HashSet<>();
        for (String line : lines) {
            String trimmed = line.strip();
            if (!trimmed.isEmpty()) {
                serials.add(trimmed.toUpperCase());
            }
        }
        return serials;
    }
}