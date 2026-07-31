-- Competition start/end boundary prices were always whatever GetLatestPrice
-- returned at the moment a worker pass happened to capture them, with
-- provider_observed_at simply stamped as that capture time (see
-- observations.go's captureObservations before this migration). A worker
-- pass can run minutes after starts_at/ends_at, or — on a retried pass —
-- days later, so the stored "boundary" price was never actually guaranteed
-- to represent the declared instant; it was a scheduling timestamp
-- masquerading as a valuation timestamp.
--
-- The application-side fix (see prices.HistoricalPriceProvider,
-- captureSinglePrice) now prefers the provider's price history at-or-before
-- the boundary instant when a historical provider is configured, and only
-- falls back to a live "whatever the market is doing right now" quote when
-- it isn't. price_methodology records which path produced the stored price
-- so that distinction is auditable per row instead of assumed:
--   'session_close'    - the close of the trading session at or before the
--                        boundary instant (the finest resolution the wired
--                        history source, Twelve Data daily bars, offers)
--   'fallback_latest'  - no historical provider was available/succeeded;
--                        this is the pre-existing "whatever GetLatestPrice
--                        returns right now" behavior, now explicit
--   'exact'            - reserved for a future intraday-capable provider
--
-- provider_observed_at now stores the PROVIDER's own timestamp for the price
-- used (the trading session's date for 'session_close', the live-fetch
-- instant for 'fallback_latest') rather than the worker's capture time.
ALTER TABLE competition_price_observations
    ADD COLUMN IF NOT EXISTS price_methodology TEXT NOT NULL DEFAULT 'fallback_latest'
        CHECK (price_methodology IN ('exact', 'session_close', 'fallback_latest')),
    ADD COLUMN IF NOT EXISTS trading_session_date DATE;
