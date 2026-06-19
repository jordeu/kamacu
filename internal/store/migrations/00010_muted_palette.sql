-- +goose Up
-- Phase 19 UAT: the original avatar palette (00009 backfill + create-time picks)
-- was too bright against the dark sidebar. icons.go projectPalette is now a muted,
-- desaturated set; this remaps every pre-existing project's icon_color from the
-- old bright hex to the new muted hex at the SAME palette position (red->muted-red,
-- etc.) so existing projects keep their hue identity but adopt the muted tone.
-- Positional remap; lower() guards stored-case drift. Rows already on a muted hex
-- (or any non-palette value) are untouched. SQL-only by design (no Go logic here).
UPDATE projects SET icon_color = '#9e5757' WHERE lower(icon_color) = '#dc2626';
UPDATE projects SET icon_color = '#9c6b4b' WHERE lower(icon_color) = '#ea580c';
UPDATE projects SET icon_color = '#8a7345' WHERE lower(icon_color) = '#d97706';
UPDATE projects SET icon_color = '#4e7a54' WHERE lower(icon_color) = '#16a34a';
UPDATE projects SET icon_color = '#46776f' WHERE lower(icon_color) = '#0d9488';
UPDATE projects SET icon_color = '#5e719c' WHERE lower(icon_color) = '#2563eb';
UPDATE projects SET icon_color = '#6c6699' WHERE lower(icon_color) = '#4f46e5';
UPDATE projects SET icon_color = '#84689e' WHERE lower(icon_color) = '#7c3aed';
UPDATE projects SET icon_color = '#9c6188' WHERE lower(icon_color) = '#db2777';

-- +goose Down
-- Reverse the positional remap back to the original bright palette.
UPDATE projects SET icon_color = '#dc2626' WHERE lower(icon_color) = '#9e5757';
UPDATE projects SET icon_color = '#ea580c' WHERE lower(icon_color) = '#9c6b4b';
UPDATE projects SET icon_color = '#d97706' WHERE lower(icon_color) = '#8a7345';
UPDATE projects SET icon_color = '#16a34a' WHERE lower(icon_color) = '#4e7a54';
UPDATE projects SET icon_color = '#0d9488' WHERE lower(icon_color) = '#46776f';
UPDATE projects SET icon_color = '#2563eb' WHERE lower(icon_color) = '#5e719c';
UPDATE projects SET icon_color = '#4f46e5' WHERE lower(icon_color) = '#6c6699';
UPDATE projects SET icon_color = '#7c3aed' WHERE lower(icon_color) = '#84689e';
UPDATE projects SET icon_color = '#db2777' WHERE lower(icon_color) = '#9c6188';
