import { Lock } from "lucide-react";
import { Link } from "react-router-dom";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

interface LockedFeatureProps {
  locked: boolean;
  feature: string;
  children: React.ReactNode;
}

export function LockedFeature({ locked, feature, children }: LockedFeatureProps) {
  if (!locked) {
    return <>{children}</>;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="relative cursor-not-allowed">
          <div className="pointer-events-none opacity-50">{children}</div>
          <div className="absolute inset-0 flex items-center justify-center">
            <Lock className="h-4 w-4 text-muted-foreground" />
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent side="top" className="max-w-xs text-center">
        <p className="font-medium">Upgrade to unlock</p>
        <p className="text-xs opacity-80">
          {feature} requires a paid license.{" "}
          <Link to="/license" className="underline">
            Upload a license key
          </Link>{" "}
          in Settings &gt; License.
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
