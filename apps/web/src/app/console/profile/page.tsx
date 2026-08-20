"use client";

import { FormEvent, useEffect, useState } from "react";
import { Check, Copy, Gift, PanelRightOpen, Save, Share2, UserPlus } from "lucide-react";

import { useCurrentUser } from "@/components/console-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { copyText } from "@/lib/clipboard";

type ReferralInvitation = {
  username: string;
  display_name: string;
  joined_at: string;
};

type ReferralReward = {
  username: string;
  display_name: string;
  paid_amount_micros: number;
  reward_micros: number;
  credited_at: string;
};

type ReferralSummary = {
  invite_code: string;
  invite_url: string;
  invited_count: number;
  pending_reward_micros: number;
  total_reward_micros: number;
  reward_bps: number;
  invitations: ReferralInvitation[];
  rewards: ReferralReward[];
};

type ReferralResponse = { referral?: ReferralSummary };

/**
 * formatMoney 封装该名称对应的业务处理逻辑。
 * @param micros 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatMoney(micros: number) {
  return new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(micros / 1_000_000);
}

/**
 * formatRewardRate 封装该名称对应的业务处理逻辑。
 * @param basisPoints 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatRewardRate(basisPoints: number) {
  return new Intl.NumberFormat("zh-CN", { style: "percent", maximumFractionDigits: 2 }).format(basisPoints / 10_000);
}

/**
 * formatDate 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

/**
 * ProfilePage 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function ProfilePage() {
  const user = useCurrentUser();
  const [displayName, setDisplayName] = useState(user.display_name);
  const [saving, setSaving] = useState(false);
  const [saveMessage, setSaveMessage] = useState("");
  const [saveSucceeded, setSaveSucceeded] = useState(false);
  const [referral, setReferral] = useState<ReferralSummary | null>(null);
  const [referralError, setReferralError] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let active = true;
    void fetch("/api/account/referral", { cache: "no-store", credentials: "same-origin" })
      .then(async (response) => {
        if (!response.ok) throw new Error();
        return response.json() as Promise<ReferralResponse>;
      })
      .then((body) => {
        if (!active || !body.referral) return;
        setReferral(body.referral);
      })
      .catch(() => active && setReferralError("邀请数据暂时无法加载"));
    return () => { active = false; };
  }, []);

  /**
   * save 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setSaveMessage("");
    setSaveSucceeded(false);
    try {
      const response = await fetch("/api/account/profile", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ display_name: displayName }),
      });
      if (!response.ok) {
        setSaveMessage("资料保存失败，请稍后重试");
        return;
      }
      setSaveSucceeded(true);
      setSaveMessage("资料已更新");
    } catch {
      setSaveMessage("资料保存失败，请稍后重试");
    } finally {
      setSaving(false);
    }
  }

  /**
   * copyInviteLink 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function copyInviteLink() {
    if (!referral?.invite_url) return;
    const success = await copyText(referral.invite_url);
    if (success) {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } else {
      setCopied(false);
    }
  }

  return (
    <div className="mx-auto max-w-6xl space-y-5">
      <div>
        <p className="text-sm text-muted-foreground">用于识别账户和调用归属</p>
        <h2 className="mt-1 text-2xl font-semibold">个人资料</h2>
      </div>

      <Card className="w-full max-w-2xl min-w-0">
        <CardHeader>
          <CardTitle className="text-base">基本信息</CardTitle>
          <CardDescription>用户名和邮箱由系统维护，显示名称可以随时修改。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-5" onSubmit={save}>
            <div className="space-y-2">
              <Label htmlFor="profile-username">用户名</Label>
              <Input className="w-full min-w-0" disabled id="profile-username" value={user.username} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="profile-email">邮箱</Label>
              <Input className="w-full min-w-0" disabled id="profile-email" value={user.email || "未设置邮箱"} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="profile-display-name">显示名称（选填）</Label>
              <Input aria-describedby="profile-display-name-hint" className="w-full min-w-0" id="profile-display-name" maxLength={128} onChange={(event) => setDisplayName(event.target.value)} value={displayName} />
              <p className="text-xs text-muted-foreground" id="profile-display-name-hint">留空时使用用户名展示。</p>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant="secondary">{user.role === "admin" ? "管理员" : "普通成员"}</Badge>
              <Button disabled={saving} type="submit"><Save />{saving ? "保存中..." : "保存资料"}</Button>
              {saveMessage ? (
                <span className={saveSucceeded ? "flex items-center gap-1 text-sm text-emerald-700 dark:text-emerald-400" : "text-sm text-destructive"} role={saveSucceeded ? "status" : "alert"}>
                  {saveSucceeded ? <Check aria-hidden="true" className="size-4" /> : null}{saveMessage}
                </span>
              ) : null}
            </div>
          </form>
        </CardContent>
      </Card>

      <Sheet>
        <Card className="w-full max-w-2xl min-w-0 overflow-hidden py-0">
          <CardContent className="p-0">
            <div className="flex items-start gap-3 p-5">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-background">
                  <Share2 aria-hidden="true" className="size-4 text-muted-foreground" />
              </span>
              <div className="min-w-0 flex-1">
                <h3 className="text-base font-semibold">推荐计划</h3>
                <p className="mt-1 break-words text-sm leading-5 text-muted-foreground">
                  好友充值确认后，你可获得 {referral ? formatRewardRate(referral.reward_bps) : "--"} 返现，奖励自动转入余额。
                </p>
              </div>
              <SheetTrigger asChild>
                <Button aria-label="查看推荐详情" disabled={!referral} size="icon" title="查看推荐详情" type="button" variant="ghost">
                  <PanelRightOpen aria-hidden="true" />
                </Button>
              </SheetTrigger>
            </div>

            <div className="grid grid-cols-3 border-y bg-muted/20">
              <ReferralMetric label="待确认" value={referral ? formatMoney(referral.pending_reward_micros) : "--"} />
              <ReferralMetric label="总收入" value={referral ? formatMoney(referral.total_reward_micros) : "--"} />
              <ReferralMetric label="邀请" value={referral ? referral.invited_count.toLocaleString("zh-CN") : "--"} />
            </div>

            <div className="flex min-w-0 items-center gap-2 p-5">
                <Input
                  aria-label="邀请链接"
                  className="h-10 min-w-0 flex-1 bg-muted/30 px-3 font-mono text-sm"
                  readOnly
                  value={referral?.invite_url ?? ""}
                />
                <Button
                  aria-label={copied ? "邀请链接已复制" : "复制邀请链接"}
                  disabled={!referral}
                  onClick={copyInviteLink}
                  size="icon-lg"
                  title={copied ? "已复制" : "复制邀请链接"}
                  type="button"
                  variant="outline"
                >
                  {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
                </Button>
            </div>
            {referralError ? <p className="border-t px-5 py-3 text-sm text-destructive" role="status">{referralError}</p> : null}
            <span className="sr-only" aria-live="polite">{copied ? "邀请链接已复制" : ""}</span>
          </CardContent>
        </Card>

        <SheetContent className="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg" side="right">
          <SheetHeader className="border-b p-5 pr-12">
            <SheetTitle>推荐详情</SheetTitle>
            <SheetDescription>查看最近的邀请成员和已经到账的返现记录。</SheetDescription>
          </SheetHeader>

          <div className="grid grid-cols-3 border-b">
            <ReferralMetric label="待确认" value={referral ? formatMoney(referral.pending_reward_micros) : "--"} />
            <ReferralMetric label="总收入" value={referral ? formatMoney(referral.total_reward_micros) : "--"} />
            <ReferralMetric label="邀请" value={referral ? referral.invited_count.toLocaleString("zh-CN") : "--"} />
          </div>

          <Tabs className="min-h-0 flex-1 gap-0" defaultValue="rewards">
            <TabsList className="mx-4 mt-4 grid h-9 w-auto grid-cols-2">
              <TabsTrigger value="rewards">返现记录</TabsTrigger>
              <TabsTrigger value="invitations">邀请记录</TabsTrigger>
            </TabsList>
            <TabsContent className="min-h-0 overflow-y-auto px-4 pb-6 pt-4" value="rewards">
              <RewardList items={referral?.rewards ?? []} />
            </TabsContent>
            <TabsContent className="min-h-0 overflow-y-auto px-4 pb-6 pt-4" value="invitations">
              <InvitationList items={referral?.invitations ?? []} />
            </TabsContent>
          </Tabs>
        </SheetContent>
      </Sheet>
    </div>
  );
}

/**
 * ReferralMetric 渲染对应的 React 界面组件。
 * @param label 本次操作需要使用的输入参数。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function ReferralMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-h-20 min-w-0 flex-col justify-center px-2 py-3 text-center">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-base font-semibold tabular-nums" title={value}>{value}</p>
    </div>
  );
}

/**
 * RewardList 渲染对应的 React 界面组件。
 * @param items 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function RewardList({ items }: { items: ReferralReward[] }) {
  if (items.length === 0) {
    return <ReferralEmptyState icon={Gift} title="还没有返现记录" description="受邀好友完成充值后，返现会自动进入你的余额。" />;
  }
  return (
    <div className="divide-y">
      {items.map((item, index) => (
        <div className="flex min-w-0 items-center gap-3 py-3" key={`${item.username}-${item.credited_at}-${index}`}>
          <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-400">
            <Gift aria-hidden="true" className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">来自 {item.display_name} 的充值</p>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">@{item.username} · 充值 {formatMoney(item.paid_amount_micros)}</p>
          </div>
          <div className="shrink-0 text-right">
            <p className="text-sm font-semibold text-emerald-700 dark:text-emerald-400">+{formatMoney(item.reward_micros)}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">{formatDate(item.credited_at)}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

/**
 * InvitationList 渲染对应的 React 界面组件。
 * @param items 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function InvitationList({ items }: { items: ReferralInvitation[] }) {
  if (items.length === 0) {
    return <ReferralEmptyState icon={UserPlus} title="还没有邀请记录" description="复制推荐链接发给好友，注册成功后会出现在这里。" />;
  }
  return (
    <div className="divide-y">
      {items.map((item) => (
        <div className="flex min-w-0 items-center gap-3 py-3" key={`${item.username}-${item.joined_at}`}>
          <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <UserPlus aria-hidden="true" className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{item.display_name}</p>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">@{item.username}</p>
          </div>
          <p className="shrink-0 text-xs text-muted-foreground">{formatDate(item.joined_at)}</p>
        </div>
      ))}
    </div>
  );
}

/**
 * ReferralEmptyState 渲染对应的 React 界面组件。
 * @param icon 本次操作需要使用的输入参数。
 * @param title 本次操作需要使用的输入参数。
 * @param description 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function ReferralEmptyState({ icon: Icon, title, description }: { icon: typeof Gift; title: string; description: string }) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center px-6 text-center">
      <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Icon aria-hidden="true" className="size-4" />
      </span>
      <p className="mt-3 text-sm font-medium">{title}</p>
      <p className="mt-1 max-w-xs text-sm leading-5 text-muted-foreground">{description}</p>
    </div>
  );
}
