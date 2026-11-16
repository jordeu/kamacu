-- +goose Up
CREATE TABLE quota_extra_usage_daily (
    account TEXT NOT NULL,
    currency TEXT NOT NULL,
    day TEXT NOT NULL,
    total REAL NOT NULL,
    spent REAL NOT NULL,
    partial INTEGER NOT NULL,
    PRIMARY KEY (account, currency, day)
);

CREATE TABLE quota_credit_tracking (
    account TEXT NOT NULL,
    currency TEXT NOT NULL,
    snapshot TEXT NOT NULL,
    PRIMARY KEY (account, currency)
);

-- +goose Down
DROP TABLE quota_credit_tracking;
DROP TABLE quota_extra_usage_daily;
