CREATE TABLE IF NOT EXISTS api_keys (
  id serial primary key,
  chat_id bigint NOT NULL,
  val varchar NOT NULL
);
CREATE UNIQUE INDEX api_keys_uniq_idx ON api_keys (chat_id);

CREATE TABLE IF NOT EXISTS price_watch (
  id serial primary key,
  chat_id bigint NOT NULL,
  ts timestamp WITH time zone DEFAULT current_timestamp,
  figi varchar NOT NULL,
  ticker varchar NOT NULL,
  threshold double precision NOT NULL default 0,
  currency varchar NOT NULL,
  is_pc boolean NOT NULL default 't',
  is_permanent boolean default 'f',
  last_value double precision NOT NULL,
  current_value double precision NOT NULL
);

CREATE UNIQUE INDEX price_watch_unique_idx ON price_watch (chat_id, figi, is_permanent, is_pc);

CREATE TABLE IF NOT EXISTS prices_daily (
  id serial primary key,
  ticker varchar NOT NULL,
  value double precision NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions_price_daily (
  id serial primary key,
  chat_id bigint NOT NULL,
  threshold double precision NOT NULL
);

CREATE UNIQUE INDEX subscriptions_price_daily_unique_idx ON subscriptions_price_daily (chat_id);

CREATE TABLE IF NOT EXISTS sent_notifications (
  id serial primary key,
  chat_id bigint NOT NULL,
  ticker varchar NOT NULL,
  notification_type varchar NOT NULL DEFAULT 'price_daily',
  ts timestamp with time zone DEFAULT current_timestamp
);

CREATE UNIQUE INDEX sent_notifications_unique_idx ON sent_notifications (chat_id, ticker, notification_type);
