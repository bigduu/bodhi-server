package models

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Version struct {
	ID          string
	Version     string
	Platform    string
	Changelog   string
	DownloadURL string
	IsLatest    bool
	ForceUpdate bool
	IsDraft     bool
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var ErrVersionExists = fmt.Errorf("version already exists")
var ErrVersionNotFound = fmt.Errorf("version not found")
var ErrVersionPublished = fmt.Errorf("version is already published")

func GetLatestVersion(ctx context.Context, db *sql.DB, platform string) (*Version, error) {
	v := &Version{}
	var downloadURL sql.NullString
	var publishedAt sql.NullTime

	query := `SELECT id::text, version, platform, changelog, download_url,
	                  is_latest, force_update, is_draft, published_at, created_at, updated_at
	           FROM versions
	           WHERE is_draft = false AND published_at <= NOW()
	             AND (platform = $1 OR platform = 'all')
	           ORDER BY published_at DESC LIMIT 1`
	err := db.QueryRowContext(ctx, query, platform).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &downloadURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &publishedAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	v.DownloadURL = downloadURL.String
	if publishedAt.Valid {
		v.PublishedAt = &publishedAt.Time
	}
	return v, nil
}

func GetVersion(ctx context.Context, db *sql.DB, id string) (*Version, error) {
	v := &Version{}
	var downloadURL sql.NullString
	var publishedAt sql.NullTime

	err := db.QueryRowContext(ctx,
		`SELECT id::text, version, platform, changelog, download_url,
		        is_latest, force_update, is_draft, published_at, created_at, updated_at
		 FROM versions WHERE id = $1::uuid`, id,
	).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &downloadURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &publishedAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	v.DownloadURL = downloadURL.String
	if publishedAt.Valid {
		v.PublishedAt = &publishedAt.Time
	}
	return v, nil
}

func ListVersions(ctx context.Context, db *sql.DB) ([]*Version, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id::text, version, platform, changelog, download_url,
		        is_latest, force_update, is_draft, published_at, created_at, updated_at
		 FROM versions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Version
	for rows.Next() {
		v := &Version{}
		var downloadURL sql.NullString
		var publishedAt sql.NullTime
		rows.Scan(
			&v.ID, &v.Version, &v.Platform, &v.Changelog, &downloadURL,
			&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &publishedAt, &v.CreatedAt, &v.UpdatedAt,
		)
		v.DownloadURL = downloadURL.String
		if publishedAt.Valid {
			v.PublishedAt = &publishedAt.Time
		}
		result = append(result, v)
	}
	return result, nil
}

func CreateVersion(ctx context.Context, db *sql.DB, version, platform, changelog, downloadURL string, forceUpdate bool) (*Version, error) {
	v := &Version{}
	var pubAt sql.NullTime
	var dlURL sql.NullString

	err := db.QueryRowContext(ctx,
		`INSERT INTO versions (version, platform, changelog, download_url, force_update)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text, version, platform, changelog, download_url,
		           is_latest, force_update, is_draft, published_at, created_at, updated_at`,
		version, platform, changelog, sqlNullString(downloadURL), forceUpdate,
	).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &dlURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &pubAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKey(err) {
			return nil, ErrVersionExists
		}
		return nil, err
	}
	v.DownloadURL = dlURL.String
	if pubAt.Valid {
		v.PublishedAt = &pubAt.Time
	}
	return v, nil
}

func UpdateVersion(ctx context.Context, db *sql.DB, id string, changelog, downloadURL, platform *string, forceUpdate *bool) (*Version, error) {
	// Build dynamic SET clause
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 2 // $1 is id

	if changelog != nil {
		setClauses = append(setClauses, fmt.Sprintf("changelog = $%d", argIdx))
		args = append(args, *changelog)
		argIdx++
	}
	if downloadURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("download_url = $%d", argIdx))
		args = append(args, sqlNullString(*downloadURL))
		argIdx++
	}
	if platform != nil {
		setClauses = append(setClauses, fmt.Sprintf("platform = $%d", argIdx))
		args = append(args, *platform)
		argIdx++
	}
	if forceUpdate != nil {
		setClauses = append(setClauses, fmt.Sprintf("force_update = $%d", argIdx))
		args = append(args, *forceUpdate)
		argIdx++
	}

	if len(setClauses) == 0 {
		return GetVersion(ctx, db, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf(
		`UPDATE versions SET %s WHERE id = $1::uuid
		 RETURNING id::text, version, platform, changelog, download_url,
		           is_latest, force_update, is_draft, published_at, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)
	args = append([]interface{}{id}, args...)

	v := &Version{}
	var dlURL sql.NullString
	var pubAt sql.NullTime
	err := db.QueryRowContext(ctx, query, args...).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &dlURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &pubAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	v.DownloadURL = dlURL.String
	if pubAt.Valid {
		v.PublishedAt = &pubAt.Time
	}
	return v, nil
}

func PublishVersion(ctx context.Context, db *sql.DB, id string) (*Version, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get the version to find its platform
	var platform string
	err = tx.QueryRowContext(ctx,
		`SELECT platform FROM versions WHERE id = $1::uuid`, id,
	).Scan(&platform)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}

	// Unset is_latest on other versions of the same platform (or 'all')
	_, err = tx.ExecContext(ctx,
		`UPDATE versions SET is_latest = false
		 WHERE (platform = $1 OR platform = 'all') AND is_latest = true`,
		platform,
	)
	if err != nil {
		return nil, err
	}

	// Publish this version
	v := &Version{}
	var dlURL sql.NullString
	var pubAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`UPDATE versions SET is_draft = false, published_at = NOW(), is_latest = true, updated_at = NOW()
		 WHERE id = $1::uuid
		 RETURNING id::text, version, platform, changelog, download_url,
		           is_latest, force_update, is_draft, published_at, created_at, updated_at`,
		id,
	).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &dlURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &pubAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	v.DownloadURL = dlURL.String
	if pubAt.Valid {
		v.PublishedAt = &pubAt.Time
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return v, nil
}

func UnpublishVersion(ctx context.Context, db *sql.DB, id string) (*Version, error) {
	v := &Version{}
	var dlURL sql.NullString
	var pubAt sql.NullTime
	err := db.QueryRowContext(ctx,
		`UPDATE versions SET is_draft = true, is_latest = false, updated_at = NOW()
		 WHERE id = $1::uuid
		 RETURNING id::text, version, platform, changelog, download_url,
		           is_latest, force_update, is_draft, published_at, created_at, updated_at`,
		id,
	).Scan(
		&v.ID, &v.Version, &v.Platform, &v.Changelog, &dlURL,
		&v.IsLatest, &v.ForceUpdate, &v.IsDraft, &pubAt, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	v.DownloadURL = dlURL.String
	if pubAt.Valid {
		v.PublishedAt = &pubAt.Time
	}
	return v, nil
}

func DeleteVersion(ctx context.Context, db *sql.DB, id string) error {
	result, err := db.ExecContext(ctx,
		`DELETE FROM versions WHERE id = $1::uuid AND is_draft = true`, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrVersionNotFound
	}
	return nil
}

func sqlNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
