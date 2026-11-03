-- One row per part per board per project. Fails if duplicates already exist; find them with:
--   SELECT pcbref, partnumber, projectname, count(*) FROM parts GROUP BY 1,2,3 HAVING count(*) > 1;
CREATE UNIQUE INDEX IF NOT EXISTS parts_identity_key ON parts (pcbref, partnumber, projectname);
