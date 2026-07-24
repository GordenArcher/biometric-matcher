-- people holds biographic data only, never a template. Kept in its own
-- table rather than one wide "identities" table so a query or a backup
-- job scoped to biographic data structurally cannot pull templates along
-- with it by accident.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE people (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    full_name TEXT NOT NULL,
    date_of_birth DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ciphertext is nonce||ciphertext as produced by internal/crypto.Encryptor,
-- Postgres never sees a plaintext template, encryption happens in Go
-- before this table is touched.
CREATE TABLE biometric_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL REFERENCES people(id) ON DELETE CASCADE,
    ciphertext BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Identify needs to page through every template to build a candidate
-- batch, this index is what makes that a real query plan instead of a
-- sequential scan once the table has any real volume.
CREATE INDEX idx_biometric_templates_person_id ON biometric_templates(person_id);