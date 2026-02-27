import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useAdminKey } from "@/lib/auth";

export function LoginPage() {
  const navigate = useNavigate();
  const { setAdminKey } = useAdminKey();
  const [adminKey, setAdminKeyInput] = useState("");
  const [error, setError] = useState("");

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

  return (
    <div className="min-h-screen bg-muted/30 flex items-center justify-center px-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Admin access</CardTitle>
          <CardDescription>
            Enter your admin key. This key is kept in memory only and is cleared on refresh.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="admin-key">Admin key</Label>
              <Input
                id="admin-key"
                placeholder="admin_demo_key"
                value={adminKey}
                onChange={(event) => setAdminKeyInput(event.target.value)}
              />
            </div>
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <Button type="submit" className="w-full">
              Continue
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
