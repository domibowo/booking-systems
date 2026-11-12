package booking

import (
	"fmt"

	"gorm.io/gorm"
)

// EnsureSchema adds what GORM's AutoMigrate can't express: the PostgreSQL
// EXCLUDE constraint that is the actual double-booking guarantee.
//
// Why an exclusion constraint instead of application-level row locking or a
// SELECT-then-INSERT check inside a transaction: those approaches only
// prevent conflicts between requests that happen to serialize through the
// same lock, and are easy to get subtly wrong under real concurrency (e.g.
// locking the wrong row, or locking a row that doesn't exist yet for a free
// slot). An EXCLUDE USING gist constraint is enforced by Postgres itself as
// part of the index structure — two overlapping ranges for the same
// resource literally cannot both commit, no matter how the application
// orders its reads and writes. It's the same mechanism recommended in the
// PostgreSQL docs for scheduling/booking systems.
//
// tstzrange(start_at, blocked_until, '[)') bakes the buffer-between-bookings
// requirement into the same check: two bookings for the same resource can't
// commit if their [start_at, blocked_until) ranges overlap, which is a
// strictly larger condition than "times overlap" whenever buffer_minutes > 0.
//
// This can't be expressed as a GORM struct tag, and can't use NOW() in the
// predicate (index predicates must be immutable) — see Create in
// repository.go for how stale pending holds are swept before each insert so
// they don't wrongly block new bookings.
func EnsureSchema(db *gorm.DB) error {
	if err := db.Exec(`CREATE EXTENSION IF NOT EXISTS btree_gist`).Error; err != nil {
		return fmt.Errorf("booking: enable btree_gist: %w", err)
	}

	var exists bool
	if err := db.Raw(`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'bookings_no_overlap')`).Scan(&exists).Error; err != nil {
		return fmt.Errorf("booking: check exclusion constraint: %w", err)
	}
	if !exists {
		if err := db.Exec(`
			ALTER TABLE bookings
			ADD CONSTRAINT bookings_no_overlap
			EXCLUDE USING gist (
				resource_id WITH =,
				tstzrange(start_at, blocked_until, '[)') WITH &&
			)
			WHERE (status IN ('pending', 'confirmed'))
		`).Error; err != nil {
			return fmt.Errorf("booking: add exclusion constraint: %w", err)
		}
	}

	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_customer_idempotency_key
		ON bookings (customer_id, idempotency_key)
		WHERE idempotency_key IS NOT NULL
	`).Error; err != nil {
		return fmt.Errorf("booking: add idempotency key index: %w", err)
	}

	return nil
}
