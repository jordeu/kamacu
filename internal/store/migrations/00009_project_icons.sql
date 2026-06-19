-- +goose Up
-- v1.7 project-icon identity (D-07). Two TEXT columns: icon_letters (the 1-2
-- uppercase monogram derived from the project name) and icon_color (a curated
-- palette hex, #rrggbb). SQL-ONLY by design — NO letter/color logic lives here.
-- Both default '' (NOT NULL DEFAULT '' is REQUIRED by SQLite ADD COLUMN and
-- leaves every pre-existing row non-null-but-blank). The idempotent Go backfill
-- (api.BackfillProjectIcons, D-08) fills those blanks right after Migrate using
-- the SAME deriveLetters+pickColor the create paths use (no divergence).
ALTER TABLE projects ADD COLUMN icon_letters TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN icon_color TEXT NOT NULL DEFAULT '';

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild) — same as 00007's/00008's Down. One DROP per statement,
-- reverse order of the adds.
ALTER TABLE projects DROP COLUMN icon_color;
ALTER TABLE projects DROP COLUMN icon_letters;
