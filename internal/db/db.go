package db

import (
	"time"

	"github.com/jackc/pgx"
	"github.com/pkg/errors"
	"github.com/triamazikamno/tinkoff-invest/pkg/pricewatch"
	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

type Database struct {
	pg *pgx.ConnPool
}

func NewDatabase(pg *pgx.ConnPool) Database {
	db := Database{
		pg: pg,
	}
	return db
}

func (db Database) IsSet() bool {
	return db.pg != nil
}

func (db Database) ApiKey(chatID int64) (string, error) {
	var key string
	err := db.pg.QueryRow(`SELECT val FROM api_keys WHERE chat_id=$1`, chatID).Scan(&key)
	return key, errors.Wrap(err, "query failed")
}

func (db Database) DeleteApiKey(chatID int64) error {
	_, err := db.pg.Exec(`DELETE FROM api_keys WHERE chat_id=$1`, chatID)
	return errors.Wrap(err, "query failed")
}

func (db Database) SetApiKey(chatID int64, key string) error {
	_, err := db.pg.Exec(
		`INSERT INTO api_keys (chat_id, val) VALUES ($1,$2) ON CONFLICT(chat_id) DO UPDATE SET val=$2`,
		chatID, key,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchAdd(chatID int64, pw pricewatch.PriceWatch) error {
	_, err := db.pg.Exec(`INSERT INTO price_watch
		(chat_id, figi, ticker, last_value, threshold, is_permanent, currency, is_pc, current_value)
		VALUES
		($1, $2, $3, $4, $5, 't', $6, $7, $4)
		ON CONFLICT (chat_id, figi, is_permanent, is_pc)
		DO UPDATE SET threshold=$5, ts=NOW(), last_value=$4, current_value=$4`,
		chatID, pw.FIGI, pw.Ticker, pw.LastValue, pw.Threshold, pw.Currency, pw.IsPc,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchSetCurrentValue(figi string, value float64) error {
	_, err := db.pg.Exec(`UPDATE price_watch SET current_value=$1 WHERE figi=$2`,
		value, figi,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchSetLastValue(id int64, value float64) error {
	_, err := db.pg.Exec(`UPDATE price_watch SET last_value=$1 WHERE id=$2`,
		value, id,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchDelete(chatID int64, figi string) error {
	_, err := db.pg.Exec(`DELETE FROM price_watch WHERE chat_id=$1 AND figi=$2`, chatID, figi)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchDeleteAll(chatID int64) error {
	_, err := db.pg.Exec(`DELETE FROM price_watch WHERE chat_id=$1`, chatID)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchDeleteByID(id int64) error {
	_, err := db.pg.Exec(`DELETE FROM price_watch WHERE id=$1`, id)
	return errors.Wrap(err, "query failed")
}

func (db Database) PriceWatchList(chatID int64) ([]pricewatch.PriceWatch, error) {
	var rows *pgx.Rows
	var err error
	if chatID == 0 {
		rows, err = db.pg.Query(
			`SELECT id, chat_id, figi, ticker, last_value, current_value, threshold, is_permanent, currency, is_pc
		FROM price_watch
		WHERE is_permanent=true`,
		)
	} else {
		rows, err = db.pg.Query(
			`SELECT id, chat_id, figi, ticker, last_value, current_value, threshold, is_permanent, currency, is_pc
		FROM price_watch
		WHERE chat_id=$1 AND is_permanent=true`,
			chatID,
		)
	}
	if err != nil {
		return nil, errors.Wrap(err, "query failed")
	}
	defer rows.Close()
	items := make([]pricewatch.PriceWatch, 0)
	for rows.Next() {
		var pw pricewatch.PriceWatch
		var currency string
		err = rows.Scan(
			&pw.ID, &pw.ChatID, &pw.FIGI, &pw.Ticker, &pw.LastValue, &pw.CurrentValue,
			&pw.Threshold, &pw.IsPermanent, &currency, &pw.IsPc,
		)
		pw.Currency = tinkoffinvest.Currency(currency)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}
		items = append(items, pw)
	}
	return items, nil
}

func (db Database) PriceWatchListByFIGI(chatID int64, figi string) ([]pricewatch.PriceWatch, error) {
	rows, err := db.pg.Query(
		`SELECT id, chat_id, figi, ticker, last_value, current_value, threshold, is_permanent, currency, is_pc
		FROM price_watch
		WHERE chat_id=$1 AND is_permanent=true AND figi=$2`,
		chatID,
		figi,
	)
	if err != nil {
		return nil, errors.Wrap(err, "query failed")
	}
	defer rows.Close()
	items := make([]pricewatch.PriceWatch, 0)
	for rows.Next() {
		var pw pricewatch.PriceWatch
		var currency string
		err = rows.Scan(
			&pw.ID, &pw.ChatID, &pw.FIGI, &pw.Ticker, &pw.LastValue, &pw.CurrentValue,
			&pw.Threshold, &pw.IsPermanent, &currency, &pw.IsPc,
		)
		pw.Currency = tinkoffinvest.Currency(currency)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}
		items = append(items, pw)
	}
	return items, nil
}

func (db Database) SubscribePriceDaily(chatID int64, threshold float64) error {
	_, err := db.pg.Exec(
		`INSERT INTO subscriptions_price_daily
		(chat_id, threshold) VALUES ($1,$2) ON CONFLICT(chat_id) DO UPDATE SET threshold=$2`,
		chatID, threshold,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) UnSubscribePriceDaily(chatID int64) error {
	_, err := db.pg.Exec(
		`DELETE FROM subscriptions_price_daily WHERE chat_id=$1`,
		chatID,
	)
	return errors.Wrap(err, "query failed")
}

func (db Database) SubscriptionsPriceDaily() (map[int64]float64, error) {
	rows, err := db.pg.Query(
		`SELECT chat_id, threshold FROM subscriptions_price_daily`,
	)
	if err != nil {
		return nil, errors.Wrap(err, "query failed")
	}
	defer rows.Close()
	items := make(map[int64]float64)
	for rows.Next() {
		var chatID int64
		var threshold float64
		err = rows.Scan(&chatID, &threshold)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}
		items[chatID] = threshold
	}
	return items, nil
}

const NotificationTypePriceDaily = "price_daily"

// PriceDailyMarkNotified marks watcher as notified. Returns true if it was already marked in the current session.
func (db Database) PriceDailyMarkNotified(chatID int64, ticker string) (bool, error) {
	var ts time.Time
	err := db.pg.QueryRow(
		`SELECT ts FROM sent_notifications WHERE chat_id=$1 AND ticker=$2 AND notification_type=$3`,
		chatID, ticker, NotificationTypePriceDaily,
	).Scan(&ts)
	if err != nil && err != pgx.ErrNoRows {
		return false, errors.Wrap(err, "query failed")
	}
	if ts.After(sessionStartTime()) {
		return true, nil
	}
	_, err = db.pg.Exec(
		`INSERT INTO sent_notifications
		(chat_id, ticker, notification_type) VALUES ($1,$2,$3)
		ON CONFLICT (chat_id, ticker, notification_type) DO UPDATE SET ts=NOW()`,
		chatID, ticker, NotificationTypePriceDaily,
	)
	if err != nil {
		return false, errors.Wrap(err, "failed to insert")
	}
	return false, nil
}

func sessionStartTime() time.Time {
	now := time.Now().UTC()
	sessionStart := now.Truncate(24 * time.Hour).Add(7 * time.Hour)
	if now.Before(sessionStart) {
		sessionStart = sessionStart.Add(-24 * time.Hour)
	}
	return sessionStart.Local()
}
