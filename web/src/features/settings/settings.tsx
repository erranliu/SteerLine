import { useState } from "react";
import { DeveloperSettings } from "./developer-settings";
import { BillingSettings } from "./billing-settings";
import { SteerAgentSettings } from "./steer-agent-settings";
import { UsageSettings } from "./usage-settings";
import { NavigationGuard } from "../../shared/ui/navigation-guard";
import {
  GenerationSettings,
  type GenerationSettingsProps,
} from "./generation-settings";

export function SettingsPage(settings: GenerationSettingsProps) {
  const [generationDirty, setGenerationDirty] = useState(false);
  const [developerDirty, setDeveloperDirty] = useState(false);
  const [steerAgentDirty, setSteerAgentDirty] = useState(false);
  return (
    <>
      <NavigationGuard
        dirty={generationDirty || developerDirty || steerAgentDirty}
      />
      <div className="page-heading">
        <h1>通用设置</h1>
      </div>
      <div className="configuration-panels">
        <SteerAgentSettings {...settings} onDirtyChange={setSteerAgentDirty} />
        <GenerationSettings {...settings} onDirtyChange={setGenerationDirty} />
        <DeveloperSettings {...settings} onDirtyChange={setDeveloperDirty} />
      </div>
    </>
  );
}

export function BillingPage(settings: GenerationSettingsProps) {
  const [billingDirty, setBillingDirty] = useState(false);
  const [usageDirty, setUsageDirty] = useState(false);
  return (
    <>
      <NavigationGuard dirty={billingDirty || usageDirty} />
      <div className="page-heading">
        <div>
          <h1>计费</h1>
          <p>所有项目共享 · 按配置模型估算费用</p>
        </div>
      </div>
      <div className="configuration-panels billing-page">
        <BillingSettings {...settings} onDirtyChange={setBillingDirty} />
        <UsageSettings {...settings} onDirtyChange={setUsageDirty} />
      </div>
    </>
  );
}
