CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE price_categories (
    id        INTEGER PRIMARY KEY,
    name      TEXT NOT NULL,
    hour_rate INTEGER NOT NULL DEFAULT 0, -- net, grosze
    day_rate  INTEGER NOT NULL DEFAULT 0, -- net, grosze
    km_rate   INTEGER NOT NULL DEFAULT 0, -- net per km over limit, grosze
    deposit   INTEGER NOT NULL DEFAULT 0, -- grosze, no VAT
    notes     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE vehicles (
    id             INTEGER PRIMARY KEY,
    kind           TEXT NOT NULL,
    make           TEXT NOT NULL,
    model          TEXT NOT NULL DEFAULT '',
    reg_number     TEXT NOT NULL DEFAULT '',
    vin            TEXT NOT NULL DEFAULT '',
    year           INTEGER NOT NULL DEFAULT 0,
    color          TEXT NOT NULL DEFAULT '',
    mileage        INTEGER NOT NULL DEFAULT 0,
    category_id    INTEGER NOT NULL REFERENCES price_categories(id),
    inspection_due TEXT NOT NULL DEFAULT '',
    oc_due         TEXT NOT NULL DEFAULT '',
    ac_due         TEXT NOT NULL DEFAULT '',
    active         INTEGER NOT NULL DEFAULT 1,
    notes          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE vehicle_blocks (
    id         INTEGER PRIMARY KEY,
    vehicle_id INTEGER NOT NULL REFERENCES vehicles(id) ON DELETE CASCADE,
    start_at   TEXT NOT NULL,
    end_at     TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX vehicle_blocks_vehicle ON vehicle_blocks(vehicle_id, start_at);

CREATE TABLE equipment (
    id     INTEGER PRIMARY KEY,
    name   TEXT NOT NULL,
    stock  INTEGER NOT NULL DEFAULT 1,
    price  INTEGER NOT NULL DEFAULT 0, -- net, one-time, grosze
    active INTEGER NOT NULL DEFAULT 1,
    notes  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE customers (
    id             INTEGER PRIMARY KEY,
    kind           TEXT NOT NULL DEFAULT 'person', -- person | company
    first_name     TEXT NOT NULL DEFAULT '',
    last_name      TEXT NOT NULL DEFAULT '',
    company_name   TEXT NOT NULL DEFAULT '',
    nip            TEXT NOT NULL DEFAULT '',
    pesel          TEXT NOT NULL DEFAULT '',
    id_document    TEXT NOT NULL DEFAULT '',
    license_number TEXT NOT NULL DEFAULT '',
    license_expiry TEXT NOT NULL DEFAULT '',
    phone          TEXT NOT NULL DEFAULT '',
    email          TEXT NOT NULL DEFAULT '',
    address        TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);

CREATE TABLE rentals (
    id                INTEGER PRIMARY KEY,
    number            TEXT NOT NULL UNIQUE,
    customer_id       INTEGER NOT NULL REFERENCES customers(id),
    vehicle_id        INTEGER NOT NULL REFERENCES vehicles(id),
    start_at          TEXT NOT NULL,
    end_at            TEXT NOT NULL,
    billing           TEXT NOT NULL DEFAULT 'day', -- day | hour
    km_limit          INTEGER,                     -- NULL = no limit
    status            TEXT NOT NULL DEFAULT 'reserved', -- reserved | issued | returned | cancelled
    -- prices copied from the price list (frozen once the vehicle is issued)
    hour_rate         INTEGER NOT NULL DEFAULT 0,
    day_rate          INTEGER NOT NULL DEFAULT 0,
    km_rate           INTEGER NOT NULL DEFAULT 0,
    vat_rate          INTEGER NOT NULL DEFAULT 2300, -- basis points
    tolerance_minutes INTEGER NOT NULL DEFAULT 60,
    deposit           INTEGER NOT NULL DEFAULT 0,
    deposit_collected INTEGER NOT NULL DEFAULT 0,
    deposit_returned  INTEGER NOT NULL DEFAULT 0, -- amount given back
    issued_at         TEXT NOT NULL DEFAULT '',
    mileage_out       INTEGER,
    fuel_out          TEXT NOT NULL DEFAULT '',
    condition_out     TEXT NOT NULL DEFAULT '',
    returned_at       TEXT NOT NULL DEFAULT '',
    mileage_in        INTEGER,
    fuel_in           TEXT NOT NULL DEFAULT '',
    condition_in      TEXT NOT NULL DEFAULT '',
    notes             TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL
);
CREATE INDEX rentals_vehicle ON rentals(vehicle_id, start_at);
CREATE INDEX rentals_customer ON rentals(customer_id);

CREATE TABLE rental_equipment (
    rental_id    INTEGER NOT NULL REFERENCES rentals(id) ON DELETE CASCADE,
    equipment_id INTEGER NOT NULL REFERENCES equipment(id),
    quantity     INTEGER NOT NULL,
    unit_price   INTEGER NOT NULL, -- net, copied from equipment
    PRIMARY KEY (rental_id, equipment_id)
);
