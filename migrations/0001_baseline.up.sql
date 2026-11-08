-- Baseline for fresh databases. No-op where the tables already exist.
CREATE TABLE IF NOT EXISTS users (
    username  text NOT NULL,
    password  text NOT NULL,
    firstname text NOT NULL DEFAULT '',
    surname   text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS parts (
    id           bigserial PRIMARY KEY,
    partnumber   text NOT NULL DEFAULT '',
    pcbref       text NOT NULL DEFAULT '',
    description  text NOT NULL DEFAULT '',
    footprint    text NOT NULL DEFAULT '',
    fit          text NOT NULL DEFAULT '',
    manufacturer text NOT NULL DEFAULT '',
    type         text NOT NULL DEFAULT '',
    projectname  text NOT NULL DEFAULT '',
    value        text NOT NULL DEFAULT ''
);
