import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  KeyRound,
  Upload,
  Trash2,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Lock,
} from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { useAdminKey } from "@/lib/auth";
import { useEntitlements } from "@/lib/entitlements";
import { scopeSettingsRead, scopeSettingsWrite } from "@/lib/scopes";
import {
  getLicenseStatus,
  uploadLicenseKey,
  removeLicenseKey,
  type LicenseStatus,
} from "@/lib/api";

export function LicensePage() {
  const { adminKey, hasScope } = useAdminKey();
  const { refresh: refreshEntitlements } = useEntitlements();
  const canRead = hasScope(scopeSettingsRead);
  const canWrite = hasScope(scopeSettingsWrite);

  const [license, setLicense] = useState<LicenseStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [actionError, setActionError] = useState("");
  const [uploading, setUploading] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const loadLicense = useCallback(async () => {
    if (!adminKey || !canRead) {
      setLoading(false);
      return;
    }
    try {
      const data = await getLicenseStatus({ adminKey });
      setLicense(data);
      setError("");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Failed to load license status.",
      );
    } finally {
      setLoading(false);
    }
  }, [adminKey, canRead]);

  useEffect(() => {
    void loadLicense();
  }, [loadLicense]);

  const handleFileSelect = async (file: File) => {
    if (!adminKey || !canWrite) return;
    setActionError("");
    setUploading(true);
    try {
      const result = await uploadLicenseKey({ adminKey }, file);
      setLicense({
        valid: result.valid,
        tier: result.tier,
        licensee: result.licensee,
        email: result.email,
        key_version: result.key_version,
        expires_at: result.expires_at,
        days_remaining: result.days_remaining,
      });
      refreshEntitlements();
      toast.success("License key activated", {
        description: `Licensed to ${result.licensee} (${result.tier} tier)`,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : "Upload failed.";
      // Try to parse structured error from API.
      try {
        const parsed = JSON.parse(message);
        setActionError(parsed.detail || parsed.error || message);
      } catch {
        setActionError(message);
      }
    } finally {
      setUploading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const file = e.dataTransfer.files[0];
    if (file) {
      void handleFileSelect(file);
    }
  };

  const handleRemove = async () => {
    if (!adminKey || !canWrite) return;
    setActionError("");
    setRemoving(true);
    try {
      const result = await removeLicenseKey({ adminKey }, tenantId);
      setLicense(result);
      setConfirmOpen(false);
      refreshEntitlements();
      toast.success("License removed", {
        description: "Reverted to free tier.",
      });
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Failed to remove license.",
      );
    } finally {
      setRemoving(false);
    }
  };

  if (loading) {
    return (
      <div className="p-6">
        <div className="text-sm text-muted-foreground">
          Loading license status...
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 space-y-4">
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
        <Button onClick={() => void loadLicense()}>Retry</Button>
      </div>
    );
  }

  const isValid = license?.valid ?? false;
  const tier = license?.tier ?? "free";
  const isTrial = tier === "trial";

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      <div>
        <h2 className="text-lg font-semibold">License</h2>
        <p className="text-sm text-muted-foreground">
          Manage your rbitr license key to unlock paid features.
        </p>
      </div>

      <Separator />

      {actionError && (
        <Alert variant="destructive">
          <AlertDescription>{actionError}</AlertDescription>
        </Alert>
      )}

      {/* Trial license banner */}
      {isTrial && license?.expires_at && (
        <Alert variant="default">
          <Clock className="h-4 w-4" />
          <AlertDescription>
            <strong>Trial License Active</strong> — Your trial license expires
            in <strong>{license.days_remaining}</strong> day
            {license.days_remaining !== 1 ? "s" : ""} (
            {new Date(license.expires_at).toLocaleDateString()}). All premium
            features are currently unlocked. Upload a paid license key to
            continue after expiration.
          </AlertDescription>
        </Alert>
      )}


      {/* Current license status */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <KeyRound className="h-5 w-5 text-muted-foreground" />
              <CardTitle className="text-base">Current Plan</CardTitle>
            </div>
            <Badge
              variant={
                isTrial ? "outline" : isValid ? "default" : "secondary"
              }
            >
              {tier}
            </Badge>
          </div>
          {isValid && license?.licensee && (
            <CardDescription>
              Licensed to {license.licensee}
              {license.email ? ` (${license.email})` : ""}
            </CardDescription>
          )}
        </CardHeader>
        {isValid && (
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-muted-foreground">Status</span>
                <div className="flex items-center gap-1.5 mt-0.5">
                  <CheckCircle2 className="h-3.5 w-3.5 text-green-500" />
                  <span className="font-medium">Active</span>
                </div>
              </div>
              {license?.expires_at && (
                <div>
                  <span className="text-muted-foreground">Expires</span>
                  <div className="mt-0.5 font-medium">
                    {new Date(license.expires_at).toLocaleDateString()}
                    {license.days_remaining !== undefined && (
                      <span className="text-muted-foreground font-normal ml-1">
                        ({license.days_remaining}d remaining)
                      </span>
                    )}
                  </div>
                </div>
              )}
              {license?.key_version !== undefined && (
                <div>
                  <span className="text-muted-foreground">Key Version</span>
                  <div className="mt-0.5 font-medium">
                    v{license.key_version}
                  </div>
                </div>
              )}
            </div>

            {canWrite && (
              <div className="pt-2">
                <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                  <DialogTrigger asChild>
                    <Button variant="destructive" size="sm">
                      <Trash2 className="h-3.5 w-3.5 mr-1" />
                      Remove License
                    </Button>
                  </DialogTrigger>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>Remove License Key?</DialogTitle>
                      <DialogDescription>
                        This will revert your installation to the free tier. All
                        paid features will become unavailable. You can upload a
                        new license key at any time.
                      </DialogDescription>
                    </DialogHeader>
                    <DialogFooter>
                      <Button
                        variant="outline"
                        onClick={() => setConfirmOpen(false)}
                      >
                        Cancel
                      </Button>
                      <Button
                        variant="destructive"
                        onClick={() => void handleRemove()}
                        disabled={removing}
                      >
                        {removing ? "Removing..." : "Remove License"}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              </div>
            )}
          </CardContent>
        )}
      </Card>

      {/* Upload section */}
      {canWrite && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {isValid ? "Replace License Key" : "Upload License Key"}
            </CardTitle>
            <CardDescription>
              {isValid
                ? "Upload a new license key to replace the current one."
                : "Upload a license.key file to unlock paid features."}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div
              className="border-2 border-dashed rounded-lg p-8 text-center cursor-pointer hover:border-primary/50 transition-colors"
              onDrop={handleDrop}
              onDragOver={(e) => e.preventDefault()}
              onClick={() => fileInputRef.current?.click()}
            >
              <Upload className="h-8 w-8 mx-auto text-muted-foreground mb-3" />
              <p className="text-sm font-medium">
                {uploading
                  ? "Uploading..."
                  : "Drop license.key file here or click to browse"}
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                Ed25519-signed JWT file, max 8 KB
              </p>
              <input
                ref={fileInputRef}
                type="file"
                accept=".key,.jwt,.pem"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void handleFileSelect(file);
                }}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Free tier info */}
      {!isValid && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-amber-500" />
              <CardTitle className="text-base">Free Tier Limits</CardTitle>
            </div>
          </CardHeader>
          <CardContent>
            <ul className="text-sm space-y-1.5 text-muted-foreground">
              <li>1 tenant, 1 agent, 1 active key</li>
              <li>10,000 governed actions per month</li>
              <li>7-day audit log retention</li>
              <li>No approval workflows or integrations</li>
              <li>No evidence export</li>
              <li>Custom policies supported</li>
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
