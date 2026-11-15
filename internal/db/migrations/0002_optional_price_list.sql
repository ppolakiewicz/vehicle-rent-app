-- The price list of a vehicle becomes optional: it is only a template for the
-- prices entered on each rental. Deleting a price list unlinks its vehicles.
CREATE TABLE vehicles_new (
    id             INTEGER PRIMARY KEY,
    kind           TEXT NOT NULL,
    make           TEXT NOT NULL,
    model          TEXT NOT NULL DEFAULT '',
    reg_number     TEXT NOT NULL DEFAULT '',
    vin            TEXT NOT NULL DEFAULT '',
    year           INTEGER NOT NULL DEFAULT 0,
    color          TEXT NOT NULL DEFAULT '',
    mileage        INTEGER NOT NULL DEFAULT 0,
    category_id    INTEGER REFERENCES price_categories(id) ON DELETE SET NULL,
    inspection_due TEXT NOT NULL DEFAULT '',
    oc_due         TEXT NOT NULL DEFAULT '',
    ac_due         TEXT NOT NULL DEFAULT '',
    active         INTEGER NOT NULL DEFAULT 1,
    notes          TEXT NOT NULL DEFAULT ''
);
INSERT INTO vehicles_new SELECT * FROM vehicles;
DROP TABLE vehicles;
ALTER TABLE vehicles_new RENAME TO vehicles;
