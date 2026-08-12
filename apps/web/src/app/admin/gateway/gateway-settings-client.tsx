"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { Activity, CheckCircle2, Info, Radio, RefreshCw, Save } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

type GatewaySettings = {
  sse_heartbeat_enabled: boolean;
  sse_heartbeat_interval_ms: number;
  upstream_timeout_ms: number;
  upstream_stream_idle_timeout_ms: number;
  reservation_input_token_cap: number;
  reservation_output_token_cap: number;
  updated_at?: string;
};

const defaults: GatewaySettings = {
  sse_heartbeat_enabled: true,
  sse_heartbeat_interval_ms: 15_000,
  upstream_timeout_ms: 0,
  upstream_stream_idle_timeout_ms: 0,
  reservation_input_token_cap: 16_384,
  reservation_output_token_cap: 1024,
};

async function readError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "使用系统默认值";
}

function validTimeout(value: string) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed >= 0 && (parsed === 0 || parsed >= 1_000) && parsed <= 86_400_000;
}

export default function GatewaySettingsClient() {
  const router = useRouter();
  const [config, setConfig] = useState<GatewaySettings>(defaults);
  const [form, setForm] = useState<GatewaySettings>(defaults);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/admin/gateway-settings", { cache: "no-store", credentials: "same-origin" });
      if (response.status === 401) { window.location.replace("/login"); return; }
      if (response.status === 403) { router.replace("/console"); return; }
      if (!response.ok) { setError(await readError(response)); return; }
      const next = ((await response.json()) as { gateway_settings?: GatewaySettings }).gateway_settings ?? defaults;
      setConfig(next);
      setForm(next);
    } catch {
      setError("请求设置读取失败，请稍后重试");
    } finally {
      setLoading(false);
    }
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    if (!Number.isInteger(form.sse_heartbeat_interval_ms) || form.sse_heartbeat_interval_ms < 1_000 || form.sse_heartbeat_interval_ms > 3_600_000) {
      setError("SSE 保活间隔必须在 1000 到 3600000 毫秒之间");
      return;
    }
    if (!validTimeout(String(form.upstream_timeout_ms)) || !validTimeout(String(form.upstream_stream_idle_timeout_ms))) {
      setError("上游超时必须为 0（关闭）或 1000 到 86400000 毫秒之间");
      return;
    }
    if (!Number.isInteger(form.reservation_input_token_cap) || form.reservation_input_token_cap < 1 || form.reservation_input_token_cap > 1_000_000 || !Number.isInteger(form.reservation_output_token_cap) || form.reservation_output_token_cap < 1 || form.reservation_output_token_cap > 1_000_000) {
	  setError("输入和输出预占上限必须在 1 到 1000000 Token 之间");
      return;
    }
    setSaving(true);
    try {
      const response = await fetch("/api/admin/gateway-settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(form),
      });
      if (!response.ok) { setError(await readError(response)); return; }
      const next = ((await response.json()) as { gateway_settings?: GatewaySettings }).gateway_settings ?? form;
      setConfig(next);
      setForm(next);
      setMessage("请求设置已保存，新请求会立即使用最新配置。");
    } catch {
      setError("请求设置保存失败，请稍后重试");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="mx-auto max-w-3xl space-y-5" onSubmit={save}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground"><Radio aria-hidden="true" className="size-4" /></span>
          <div>
            <div className="flex flex-wrap items-center gap-2"><p className="font-medium">请求生命周期</p><Badge variant="secondary">全局</Badge></div>
            <p className="mt-1 text-sm text-muted-foreground">控制 SSE 连接保活，以及上游模型请求的最长等待时间。</p>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button aria-label="刷新请求设置" disabled={loading || saving} onClick={() => void load()} size="icon" title="刷新请求设置" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
          <Button disabled={loading || saving} type="submit"><Save />{saving ? "保存中..." : "保存设置"}</Button>
        </div>
      </div>

      {message ? <div className="flex items-start gap-2 rounded-md border border-emerald-600/25 bg-emerald-500/5 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-400" role="status"><CheckCircle2 className="mt-0.5 size-4 shrink-0" /><span>{message}</span></div> : null}
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive" role="alert">{error}</p> : null}

      <Card>
		<CardHeader><CardTitle className="flex items-center gap-2"><Activity className="size-4 text-muted-foreground" />余额预占</CardTitle><CardDescription>预占只用于调用前的余额保护，最终费用始终按上游明确返回的 usage 结算。</CardDescription></CardHeader>
		<CardContent className="space-y-5">
		  <div className="space-y-2"><Label htmlFor="reservation-input-cap">输入预占上限 (Token)</Label><Input id="reservation-input-cap" inputMode="numeric" max={1_000_000} min={1} onChange={(event) => setForm((current) => ({ ...current, reservation_input_token_cap: Number(event.target.value) }))} type="number" value={form.reservation_input_token_cap} /><p className="text-xs text-muted-foreground">请求体估算输入高于此值时，仅按此上限预占，不限制实际上下文；默认 16384。</p></div>
		  <div className="space-y-2"><Label htmlFor="reservation-output-cap">输出预占上限 (Token)</Label><Input id="reservation-output-cap" inputMode="numeric" max={1_000_000} min={1} onChange={(event) => setForm((current) => ({ ...current, reservation_output_token_cap: Number(event.target.value) }))} type="number" value={form.reservation_output_token_cap} /><p className="text-xs text-muted-foreground">请求声明的最大输出高于此值时，仅按此上限预占。实际输出超过预占后，系统依据最终 usage 幂等补扣；默认 1024。</p></div>
		</CardContent>
	  </Card>

	  <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Radio className="size-4 text-muted-foreground" />保持连接心跳</CardTitle><CardDescription>开启后，Novro 会按间隔向已建立的 SSE 客户端发送注释心跳，减少代理或网络设备因长时间无数据而断开。</CardDescription></CardHeader>
        <CardContent className="space-y-5">
          <div className="flex items-center justify-between gap-4 rounded-md border p-4"><div><Label className="cursor-pointer" htmlFor="sse-heartbeat-enabled">启用 SSE 保活</Label><p className="mt-1 text-xs text-muted-foreground">只影响流式响应，不会修改上游模型返回的数据。</p></div><Switch aria-label="启用 SSE 保活" checked={form.sse_heartbeat_enabled} disabled={loading || saving} id="sse-heartbeat-enabled" onCheckedChange={(checked) => setForm((current) => ({ ...current, sse_heartbeat_enabled: checked }))} /></div>
          <div className="space-y-2"><Label htmlFor="sse-heartbeat-interval">SSE 保活间隔 (ms)</Label><Input id="sse-heartbeat-interval" inputMode="numeric" max={3_600_000} min={1_000} onChange={(event) => setForm((current) => ({ ...current, sse_heartbeat_interval_ms: Number(event.target.value) }))} type="number" value={form.sse_heartbeat_interval_ms} /><p className="text-xs text-muted-foreground">建议保持 15000；最小 1000，最大 3600000。</p></div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2"><Activity className="size-4 text-muted-foreground" />上游请求超时</CardTitle><CardDescription>这些限制作用于 Novro 到模型提供商的请求。填写 0 表示关闭限制。</CardDescription></CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2"><Label htmlFor="upstream-timeout">上游总超时 (ms，0 为关闭)</Label><Input id="upstream-timeout" inputMode="numeric" max={86_400_000} min={0} onChange={(event) => setForm((current) => ({ ...current, upstream_timeout_ms: Number(event.target.value) }))} type="number" value={form.upstream_timeout_ms} /><p className="text-xs text-muted-foreground">包含连接、等待响应头和读取响应体的时间，也会覆盖路由重试的总时长。</p></div>
          <div className="space-y-2"><Label htmlFor="upstream-idle-timeout">上游流式空闲超时 (ms)</Label><Input id="upstream-idle-timeout" inputMode="numeric" max={86_400_000} min={0} onChange={(event) => setForm((current) => ({ ...current, upstream_stream_idle_timeout_ms: Number(event.target.value) }))} type="number" value={form.upstream_stream_idle_timeout_ms} /><p className="text-xs text-muted-foreground">流式响应收到任意上游字节后重新计时；超过该时间没有上游数据就结束流。0 表示不限制。</p></div>
          <div className="flex items-start gap-2 rounded-md bg-muted/50 px-3 py-2 text-xs text-muted-foreground"><Info className="mt-0.5 size-4 shrink-0" /><span>连接超时和 TLS 握手超时仍由服务端固定保护；这里只控制截图中的三个请求生命周期选项。</span></div>
          <dl className="grid gap-4 border-t pt-5 text-sm sm:grid-cols-2"><div><dt className="text-muted-foreground">当前 SSE 保活</dt><dd className="mt-1 font-medium">{config.sse_heartbeat_enabled ? `已启用，每 ${config.sse_heartbeat_interval_ms} ms` : "已关闭"}</dd></div><div><dt className="text-muted-foreground">最近保存</dt><dd className="mt-1 font-medium">{formatDate(config.updated_at)}</dd></div></dl>
        </CardContent>
      </Card>
    </form>
  );
}
