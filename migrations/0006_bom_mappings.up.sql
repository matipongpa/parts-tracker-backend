-- Column mappings for BOM workbooks, confirmed by users and shared by the whole team. Owned by the bomconverter module.
-- fingerprint is a hash of the normalised header row (see layout.Fingerprint); the same columns in the
-- same places give the same fingerprint, so the next file with that layout is pre-filled.
-- created_by / updated_by are usernames from the token; no foreign key to users (module boundary).
CREATE TABLE IF NOT EXISTS bom_mappings (
    fingerprint text        PRIMARY KEY,
    mapping     jsonb       NOT NULL,
    created_by  text        NOT NULL,
    updated_by  text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
