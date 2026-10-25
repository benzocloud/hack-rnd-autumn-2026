-- +goose Up
CREATE TABLE core_scenarios (
 company_id uuid NOT NULL,
 account_id uuid NOT NULL,
 id uuid NOT NULL,
 forecast_id uuid NOT NULL,
 base_forecast_id uuid NOT NULL,
 name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL,
 payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object'),
 PRIMARY KEY(company_id,account_id,id),
 UNIQUE(company_id,account_id,forecast_id),
 FOREIGN KEY(company_id,account_id,base_forecast_id) REFERENCES core_forecast_jobs(company_id,account_id,id)
);
CREATE INDEX core_scenarios_listing ON core_scenarios(company_id,account_id,id DESC);
-- +goose Down
DROP TABLE core_scenarios;
