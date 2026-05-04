package admin

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

const (
	maxLicenseKeySize = 8 * 1024 // 8 KB — JWTs are small.
	licenseDirPerm    = 0o750
	hoursPerDay       = 24
)

// handleLicenseStatus returns the current license info.
func (d *Dependencies) handleLicenseStatus(c *echo.Context) error {
	if d.LicenseProvider == nil {
		return c.JSON(http.StatusOK, map[string]any{
			fieldValid: false,
			fieldTier:  tierFreeStr,
		})
	}

	info := d.LicenseProvider.Info()
	resp := map[string]any{
		fieldValid: info.Valid,
		fieldTier:  info.Tier,
	}

	if info.Valid {
		resp["licensee"] = info.Licensee
		resp[fieldEmail] = info.Email
		resp["key_version"] = info.KeyVersion
		resp["issued_at"] = info.IssuedAt
		resp["expires_at"] = info.ExpiresAt
		resp["days_remaining"] = daysRemaining(info.ExpiresAt)
	}

	return c.JSON(http.StatusOK, resp)
}

// handleLicenseUpload accepts a license key file, validates it, writes it to
// disk, and triggers a hot-reload. Only supported for self-managed providers.
func (d *Dependencies) handleLicenseUpload(c *echo.Context) error {
	mgr, ok := d.LicenseProvider.(license.SelfManagedManager)
	if !ok || d.LicenseProvider == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{
			"error": "license upload is only supported for self-managed installations",
		})
	}

	data, err := readLicenseBody(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Validate before writing.
	info, validateErr := mgr.ValidateBytes(data)
	if validateErr != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{
			"error":     "INVALID_LICENSE_KEY",
			fieldDetail: validateErr.Error(),
		})
	}

	// Check if trial license can be uploaded (one-time-use enforcement).
	if info.Tier == "trial" && d.Store != nil {
		hasTrialBeenUsed, checkErr := d.Store.HasTrialLicenseBeenUsed(c.Request().Context())
		if checkErr != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to check trial license history",
			})
		}
		if hasTrialBeenUsed {
			return c.JSON(http.StatusForbidden, map[string]string{
				"error":     "TRIAL_ALREADY_USED",
				fieldDetail: "Trial license can only be used once per installation. This installation has already consumed its trial period.",
			})
		}
	}

	// Write to disk atomically (write tmp then rename).
	keyPath := mgr.KeyPath()
	if writeErr := atomicWriteFile(keyPath, data); writeErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error":     "failed to write license key file",
			fieldDetail: writeErr.Error(),
		})
	}

	// Trigger hot-reload.
	mgr.LoadAndValidate()

	// Record in license history.
	fp := license.Fingerprint(data)
	if d.Store != nil {
		_ = d.Store.InsertLicenseHistory(
			c.Request().Context(),
			info.Tier,
			info.KeyVersion,
			info.Licensee,
			info.Email,
			info.ExpiresAt,
			fp,
		)
	}

	return c.JSON(http.StatusOK, map[string]any{
		fieldValid:       true,
		fieldTier:        info.Tier,
		"licensee":       info.Licensee,
		fieldEmail:       info.Email,
		"key_version":    info.KeyVersion,
		"expires_at":     info.ExpiresAt,
		"days_remaining": daysRemaining(info.ExpiresAt),
		"fingerprint":    fp,
	})
}

// handleLicenseRemove deletes the license key file and reverts to free tier.
// Only supported for self-managed providers.
func (d *Dependencies) handleLicenseRemove(c *echo.Context) error {
	mgr, ok := d.LicenseProvider.(license.SelfManagedManager)
	if !ok || d.LicenseProvider == nil {
		return c.JSON(http.StatusNotImplemented, map[string]string{
			"error": "license removal is only supported for self-managed installations",
		})
	}

	keyPath := mgr.KeyPath()
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to remove license key file",
		})
	}

	// Trigger reload — will fall back to free tier.
	mgr.LoadAndValidate()

	return c.JSON(http.StatusOK, map[string]any{
		fieldValid: false,
		fieldTier:  tierFreeStr,
	})
}

// readLicenseBody reads the license key from either multipart form upload or
// raw request body. Limited to maxLicenseKeySize.
func readLicenseBody(c *echo.Context) ([]byte, error) {
	contentType := c.Request().Header.Get("Content-Type")

	// Try multipart form upload first.
	if contentType != "" && contentType[:10] == "multipart/" {
		file, _, err := c.Request().FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("missing 'file' in multipart form: %w", err)
		}
		defer file.Close()
		return io.ReadAll(io.LimitReader(file, maxLicenseKeySize+1))
	}

	// Fall back to raw body.
	data, err := io.ReadAll(io.LimitReader(c.Request().Body, maxLicenseKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}

	if len(data) == 0 {
		return nil, errors.New("empty request body")
	}
	if len(data) > maxLicenseKeySize {
		return nil, fmt.Errorf("license key exceeds maximum size (%d bytes)", maxLicenseKeySize)
	}

	return data, nil
}

// atomicWriteFile writes data to a temp file then renames it to the target
// path, ensuring the file is either fully written or not changed.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, licenseDirPerm); mkErr != nil {
		return fmt.Errorf("creating directory %s: %w", dir, mkErr)
	}

	tmp, err := os.CreateTemp(dir, ".license-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, path); renameErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", renameErr)
	}

	return nil
}

// daysRemaining returns the number of days until expiry, or 0 if already expired.
func daysRemaining(expiresAt time.Time) int {
	d := int(time.Until(expiresAt).Hours() / hoursPerDay)
	if d < 0 {
		return 0
	}
	return d
}
