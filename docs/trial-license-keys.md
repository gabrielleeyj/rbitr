# Trial License Keys

## Overview

Trial license keys provide a controlled way to offer 14-day trial periods while preventing abuse through reinstallation. Unlike the automatic free trial (which starts when a tenant is created), trial licenses are cryptographically signed keys that can only be used once per installation.

## Why Trial License Keys?

The automatic free trial has a security gap: users can repeatedly reinstall rbitr to get unlimited 14-day trials. Trial license keys solve this by:

1. **One-time use enforcement** — A trial key can only be uploaded once per installation
2. **Controlled distribution** — You control who gets trial access via key generation
3. **Fixed expiration** — Trial keys expire 14 days from generation, not activation
4. **Audit trail** — All trial key uploads are tracked in `license_history`

## Trial Key vs. Automatic Trial

| Feature | Automatic Trial | Trial License Key |
|---------|----------------|-------------------|
| **Activation** | Starts when first tenant is created | Starts when key is uploaded |
| **Duration** | 14 days from tenant creation | 14 days from key generation |
| **Reuse** | Can be gamed via reinstallation | One-time use per installation |
| **Distribution** | Available to all installations | Controlled via key generation |
| **Entitlements** | Full paid-tier features | Full paid-tier features |

## Generating Trial License Keys

Use the `license-gen` tool with `--tier=trial`:

```bash
# Generate a trial license key
./license-gen \
  --private-key keys/private.pem \
  --licensee "Acme Corp" \
  --email "admin@acme.com" \
  --tier trial \
  --output trial-acme.key
```

**Note:** The `--expires` flag is optional for trial keys. If omitted, expiry is automatically set to 14 days from generation.

### Manual Expiry for Trial Keys

If you need a custom trial duration, specify `--expires`:

```bash
# 7-day trial instead of 14
./license-gen \
  --private-key keys/private.pem \
  --licensee "Acme Corp" \
  --email "admin@acme.com" \
  --tier trial \
  --expires 2026-04-02 \
  --output trial-acme-7day.key
```

## One-Time Use Enforcement

### How It Works

1. User uploads a trial license key via UI (Settings > License)
2. Backend validates the signature and tier
3. Backend checks `license_history` table for any previous trial uploads
4. If a trial key was uploaded before → **reject with error**
5. If this is the first trial key → **accept and record in history**

### Error Response

When a trial key is rejected due to prior use:

```json
{
  "error": "TRIAL_ALREADY_USED",
  "detail": "Trial license can only be used once per installation. This installation has already consumed its trial period."
}
```

### Database Query

The enforcement check runs this query:

```sql
SELECT EXISTS(SELECT 1 FROM rbitr.license_history WHERE tier = 'trial' LIMIT 1)
```

If this returns `true`, the trial key upload is blocked.

## License Tier Comparison

| Tier | Upload Restrictions | Feature Access | Expiration |
|------|---------------------|----------------|------------|
| **free** | N/A (no key) | Limited (no approvals, integrations) | Never |
| **trial** | One per installation | Full (same as paid) | 14 days |
| **paid** | Unlimited | Full | Per license terms |

## Trial Key Lifecycle

### 1. Key Generation
```
Day -1: Admin generates trial key with 14-day expiry
```

### 2. Key Distribution
```
Day 0: Admin sends trial-acme.key to customer
```

### 3. Key Upload
```
Day 3: Customer uploads key → activates trial
        Backend records in license_history
```

### 4. Trial Active
```
Days 3-17: All premium features unlocked
           UI shows "Trial License Active — X days remaining"
```

### 5. Trial Expires
```
Day 17: License expires → features locked
        Customer must upload paid license to continue
```

### 6. Upgrade Path
```
Day 20: Customer uploads paid license → full access restored
        (No restriction on paid license uploads)
```

## Preventing Abuse

### Attack Vector: Reinstallation

**Threat:** User reinstalls rbitr on the same machine to reset the trial.

**Mitigation:**
- Trial key expiry is based on **generation date**, not activation date
- Reinstalling doesn't extend the trial (key is already expired)
- One-time use check happens **before** writing the key to disk

### Attack Vector: Database Reset

**Threat:** User drops the database and re-initializes to clear `license_history`.

**Mitigation:**
- Deleting the database also deletes all tenants, policies, and audit logs
- This is equivalent to a fresh install (losing all work)
- For most users, this is not a viable attack vector
- Enterprise users can be required to show audit trail integrity

### Attack Vector: Key Sharing

**Threat:** User shares their trial key with others.

**Mitigation:**
- Trial keys are one-time use **per installation**
- Key can't be reused on the same installation
- Licensee name/email embedded in key for audit trail
- Can be detected via fingerprint analysis (same key on multiple installs)

## Implementation Details

### Database Schema

Trial keys are recorded in the same `license_history` table:

```sql
CREATE TABLE rbitr.license_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tier TEXT NOT NULL,              -- 'trial' for trial keys
    key_version INT NOT NULL,
    licensee TEXT NOT NULL,
    email TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    fingerprint TEXT NOT NULL        -- SHA-256 of key for dedup
);
```

### Backend Enforcement

Location: `internal/api/admin/license_management.go`

```go
// Check if trial license can be uploaded (one-time-use enforcement)
if info.Tier == "trial" && d.Store != nil {
    hasTrialBeenUsed, checkErr := d.Store.HasTrialLicenseBeenUsed(c.Request().Context())
    if checkErr != nil {
        return c.JSON(http.StatusInternalServerError, map[string]string{
            "error": "failed to check trial license history",
        })
    }
    if hasTrialBeenUsed {
        return c.JSON(http.StatusForbidden, map[string]string{
            "error":  "TRIAL_ALREADY_USED",
            "detail": "Trial license can only be used once per installation.",
        })
    }
}
```

### UI Display

Trial licenses show a distinct banner:

```
┌─────────────────────────────────────────────────┐
│ ⏱️  Trial License Active — Your trial license   │
│     expires in 11 days (4/15/2026). All premium │
│     features are currently unlocked.             │
└─────────────────────────────────────────────────┘

Current Plan: trial [outline badge]
Licensed to: Acme Corp (admin@acme.com)
Expires: 2026-04-15 (11d remaining)
Status: ● Active
```

## Best Practices

### For rbitr Administrators

1. **Generate trial keys on demand** — Don't pre-generate a batch
2. **Track key distribution** — Maintain a spreadsheet of who got which key
3. **Set reasonable expiry** — 14 days is standard, adjust if needed
4. **Monitor usage** — Watch for same fingerprint on multiple installs (key sharing)

### For Customers

1. **Upload trial key early** — Don't wait until you need it (key expires from generation, not activation)
2. **Evaluate during trial** — Test all premium features before expiry
3. **Plan upgrade path** — Contact sales before trial expires if you need more time
4. **Don't reinstall to extend** — Trial keys are time-locked to generation date

## Troubleshooting

### "Trial license can only be used once per installation"

**Cause:** This installation has already consumed its trial period.

**Solution:**
- Contact sales for a paid license key
- If you believe this is an error, check `license_history` table for prior trial uploads

### Trial key expired immediately after upload

**Cause:** The key was generated more than 14 days ago.

**Solution:**
- Request a new trial key from your sales contact
- Trial keys expire based on generation date, not upload date

### Can't upload paid license after trial

**Cause:** This is likely not the issue — paid licenses can always be uploaded.

**Check:**
- Verify the key file is valid
- Check the signature and expiry date
- Review upload error message for details

## Migration Notes

### Existing Installations

Installations that used the automatic trial (before trial license keys were implemented) can still upload trial license keys. The one-time-use check only applies to trial **license keys**, not the automatic trial system.

### Backward Compatibility

- Free tier installations: No change
- Paid license installations: No change
- Automatic trial installations: Can still upload trial keys (if none have been uploaded before)

## Related Documentation

- [EPIC 13 - Freemium Model](./EPIC_13.md)
- [Trial Implementation Summary](./trial-implementation.md)
- [License Management](../README.md#freemium-model--tier-system)
