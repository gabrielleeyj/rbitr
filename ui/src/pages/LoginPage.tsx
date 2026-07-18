import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useAdminKey } from "@/lib/auth";
import { getSSOStatus, getSSOAuthorizeURL, ssoCallback } from "@/lib/api";

export function LoginPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { setAdminKey } = useAdminKey();
  const [adminKey, setAdminKeyInput] = useState("");
  const [error, setError] = useState("");
  const [ssoEnabled, setSSOEnabled] = useState(false);
  const [ssoLoading, setSSOLoading] = useState(false);
  const [ssoChecked, setSSOChecked] = useState(false);

  // Check if SSO is enabled on mount.
  useEffect(() => {
    let mounted = true;
    getSSOStatus()
      .then((status) => {
        if (mounted) {
          setSSOEnabled(status.sso_enabled);
          setSSOChecked(true);
        }
      })
      .catch(() => {
        if (mounted) setSSOChecked(true);
      });
    return () => { mounted = false; };
  }, []);

  // Handle SSO callback (redirected back from IdP with ?code=...).
  useEffect(() => {
    const code = searchParams.get("code");
    if (!code) return;

    let mounted = true;
    setSSOLoading(true);
    setError("");

    ssoCallback(code)
      .then((resp) => {
        if (!mounted) return;
        setAdminKey(resp.token);
        navigate("/tenants");
      })
      .catch(() => {
        if (!mounted) return;
        setError("SSO authentication failed. Please try again.");
        setSSOLoading(false);
      });
    return () => { mounted = false; };
  }, [searchParams, setAdminKey, navigate]);

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    if (!adminKey.trim()) {
      setError("Admin key is required.");
      return;
    }
    setError("");
    setAdminKey(adminKey.trim());
    navigate("/tenants");
  };

  const handleSSOLogin = () => {
    setSSOLoading(true);
    setError("");
    getSSOAuthorizeURL()
      .then((resp) => {
        window.location.href = resp.authorize_url;
      })
      .catch(() => {
        setError("Failed to start SSO login. Please try again.");
        setSSOLoading(false);
      });
  };

  return (
    <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Admin access</CardTitle>
          <CardDescription>
            {ssoEnabled
              ? "Sign in with your organization SSO or enter an admin key."
              : "Enter your admin key. This key is kept in memory only and is cleared on refresh."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {ssoEnabled && ssoChecked ? (
            <>
              <Button
                className="w-full"
                onClick={handleSSOLogin}
                disabled={ssoLoading}
              >
                {ssoLoading ? "Redirecting..." : "Sign in with SSO"}
              </Button>
              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <span className="w-full border-t" />
                </div>
                <div className="relative flex justify-center text-xs uppercase">
                  <span className="bg-card px-2 text-muted-foreground">or</span>
                </div>
              </div>
            </>
          ) : null}
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="admin-key">Admin key</Label>
              <Input
                id="admin-key"
                placeholder="Paste your admin key"
                value={adminKey}
                onChange={(event) => setAdminKeyInput(event.target.value)}
              />
            </div>
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <Button type="submit" variant={ssoEnabled ? "outline" : "default"} className="w-full">
              Continue with admin key
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
