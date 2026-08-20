"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { CheckCircle2, Eye, EyeOff, KeyRound, Mail, RefreshCw, Save, Send, Server, ShieldCheck } from "lucide-react";
import { useRouter } from "next/navigation";

import { useCurrentUser } from "@/components/console-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";

type Security = "none" | "starttls" | "ssl";
type EmailConfig = {
  enabled: boolean;
  configured: boolean;
  host: string;
  port: number;
  username: string;
  from_address: string;
  security: Security;
  has_password: boolean;
  updated_at?: string;
};
type EmailConfigResponse = Partial<EmailConfig>;
type EmailForm = {
  enabled: boolean;
  host: string;
  port: string;
  username: string;
  password: string;
  from_address: string;
  security: Security;
};

const emptyConfig: EmailConfig = {
  enabled: false,
  configured: false,
  host: "",
  port: 587,
  username: "",
  from_address: "",
  security: "starttls",
  has_password: false,
};

/**
 * readError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function readError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

/**
 * normalizeConfig 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function normalizeConfig(value?: EmailConfigResponse): EmailConfig {
  const security = value?.security;
  return {
    enabled: value?.enabled === true,
    configured: value?.configured === true,
    host: value?.host ?? "",
    port: Number.isInteger(value?.port) && Number(value?.port) > 0 ? Number(value?.port) : 587,
    username: value?.username ?? "",
    from_address: value?.from_address ?? "",
    security: security === "none" || security === "ssl" ? security : "starttls",
    has_password: value?.has_password === true,
    updated_at: value?.updated_at,
  };
}

/**
 * toForm 封装该名称对应的业务处理逻辑。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function toForm(config: EmailConfig): EmailForm {
  return {
    enabled: config.enabled,
    host: config.host,
    port: String(config.port),
    username: config.username,
    password: "",
    from_address: config.from_address,
    security: config.security,
  };
}

/**
 * formatDate 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "尚未通过控制台保存";
}

/**
 * EmailClient 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function EmailClient() {
  const router = useRouter();
  const currentUser = useCurrentUser();
  const [config, setConfig] = useState<EmailConfig>(emptyConfig);
  const [form, setForm] = useState<EmailForm>(() => toForm(emptyConfig));
  const [testRecipient, setTestRecipient] = useState(currentUser.email);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [testRecipientError, setTestRecipientError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    const response = await fetch("/api/admin/email", { cache: "no-store" });
    if (response.status === 401) { router.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setError(await readError(response)); setLoading(false); return; }
    const next = normalizeConfig(((await response.json()) as { email_config?: EmailConfigResponse }).email_config);
    setConfig(next);
    setForm(toForm(next));
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  /**
   * save 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setMessage("");
    setError("");
    const port = Number(form.port);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setError("端口必须是 1 到 65535 之间的整数");
      setSaving(false);
      return;
    }
    const payload: Record<string, string | number | boolean> = {
      enabled: form.enabled,
      host: form.host.trim(),
      port,
      username: form.username.trim(),
      from_address: form.from_address.trim(),
      security: form.security,
    };
    if (form.password.trim()) payload.password = form.password;
    const response = await fetch("/api/admin/email", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!response.ok) {
      setError(await readError(response));
      setSaving(false);
      return;
    }
    const next = normalizeConfig(((await response.json()) as { email_config?: EmailConfigResponse }).email_config);
    setConfig(next);
    setForm(toForm(next));
    setMessage("SMTP 配置已保存，新的注册验证码邮件会立即使用这组设置。");
    setSaving(false);
  }

  /**
   * sendTest 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function sendTest() {
    const recipient = testRecipient.trim();
    const recipientInput = document.getElementById("smtp-test-recipient") as HTMLInputElement | null;
    if (recipientInput && !recipientInput.checkValidity()) {
      setTestRecipientError("请输入有效的测试收件地址。");
      recipientInput.reportValidity();
      recipientInput.focus();
      return;
    }
    setTestRecipientError("");
    setTesting(true);
    setMessage("");
    setError("");
    const response = await fetch("/api/admin/email/test", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ recipient }),
    });
    if (!response.ok) {
      setError(await readError(response));
      setTesting(false);
      return;
    }
    setMessage(`测试邮件已发送到 ${recipient}，请检查收件箱。`);
    setTesting(false);
  }

  const status = config.enabled && config.configured ? "运行中" : config.configured ? "已配置，未启用" : "未配置";
  const securityHint = form.security === "ssl" ? "连接建立时立即启用 TLS，常用端口为 465。" : form.security === "starttls" ? "先连接 SMTP，再升级为加密连接，常用端口为 587。" : "邮件内容和凭据不会加密，仅建议在受控开发网络使用。";

  return (
    <form className="mx-auto max-w-6xl space-y-5" onSubmit={save}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground"><Mail className="size-4" /></span>
          <div>
            <div className="flex flex-wrap items-center gap-2"><p className="font-medium">注册邮件通道</p><Badge variant={config.enabled && config.configured ? "default" : config.configured ? "secondary" : "outline"}>{status}</Badge></div>
            <p className="mt-1 text-sm text-muted-foreground">用于发送注册验证码和管理员测试邮件。</p>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button aria-label="刷新邮件配置" disabled={loading || saving} onClick={() => void load()} size="icon" title="刷新邮件配置" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
          <Button disabled={loading || saving} type="submit"><Save />{saving ? "保存中..." : "保存配置"}</Button>
        </div>
      </div>

      {message ? <div className="flex items-start gap-2 rounded-md border border-emerald-600/25 bg-emerald-500/5 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-400" role="status"><CheckCircle2 className="mt-0.5 size-4 shrink-0" /><span>{message}</span></div> : null}
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive" role="alert">{error}</p> : null}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <div className="space-y-5">
          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2"><Server className="size-4 text-muted-foreground" />SMTP 连接</CardTitle><CardDescription>填写服务商提供的主机、端口和传输加密方式。</CardDescription></CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_140px]">
              <div className="space-y-2"><Label htmlFor="smtp-host">SMTP 主机</Label><Input autoComplete="off" id="smtp-host" maxLength={255} onChange={(event) => setForm({ ...form, host: event.target.value })} placeholder="smtp.example.com" required={form.enabled} value={form.host} /><p className="text-xs text-muted-foreground">仅填写主机名或 IP，不包含协议和端口。</p></div>
              <div className="space-y-2"><Label htmlFor="smtp-port">端口</Label><Input id="smtp-port" inputMode="numeric" max={65535} min={1} onChange={(event) => setForm({ ...form, port: event.target.value })} required type="number" value={form.port} /><p className="text-xs text-muted-foreground">常用 25、465、587</p></div>
              <div className="space-y-2 sm:col-span-2"><Label htmlFor="smtp-security">加密方式</Label><Select onValueChange={(value) => setForm({ ...form, security: value as Security })} value={form.security}><SelectTrigger className="w-full" id="smtp-security"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="starttls">STARTTLS（推荐）</SelectItem><SelectItem value="ssl">SSL/TLS</SelectItem><SelectItem value="none">无加密</SelectItem></SelectContent></Select><p className="text-xs text-muted-foreground">{securityHint}</p></div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4 text-muted-foreground" />发件身份与认证</CardTitle><CardDescription>凭据会加密保存；已保存的密码不会返回浏览器。</CardDescription></CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label htmlFor="smtp-username">用户名</Label><Input autoComplete="username" id="smtp-username" maxLength={320} onChange={(event) => setForm({ ...form, username: event.target.value })} placeholder="verify@example.com" required={form.enabled} value={form.username} /></div>
              <div className="space-y-2"><Label htmlFor="smtp-from">发件地址</Label><Input autoComplete="email" id="smtp-from" maxLength={320} onChange={(event) => setForm({ ...form, from_address: event.target.value })} placeholder="verify@example.com" required={form.enabled} type="email" value={form.from_address} /></div>
              <div className="space-y-2 sm:col-span-2"><Label htmlFor="smtp-password">密码 / 访问令牌</Label><div className="relative"><KeyRound className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input autoComplete="new-password" className="pl-9 pr-10" id="smtp-password" onChange={(event) => setForm({ ...form, password: event.target.value })} placeholder={config.has_password ? "已安全保存，留空保持不变" : "输入 SMTP 密码或授权码"} required={form.enabled && !config.has_password} type={showPassword ? "text" : "password"} value={form.password} /><Button aria-label={showPassword ? "隐藏 SMTP 密码" : "显示 SMTP 密码"} className="absolute right-1 top-1/2 -translate-y-1/2" onClick={() => setShowPassword((value) => !value)} size="icon-sm" title={showPassword ? "隐藏密码" : "显示密码"} type="button" variant="ghost">{showPassword ? <EyeOff /> : <Eye />}</Button></div><p className="text-xs text-muted-foreground">{config.has_password ? "服务器已有凭据；只有输入新值时才会替换。" : "多数邮箱服务需要使用单独生成的 SMTP 授权码。"}</p></div>
            </CardContent>
          </Card>
        </div>

        <div className="space-y-5">
          <Card>
            <CardHeader><CardTitle>发送状态</CardTitle><CardDescription>关闭后将停止通过 SMTP 发送新的注册验证码。</CardDescription></CardHeader>
            <CardContent className="space-y-4"><div className="flex items-center justify-between gap-4 rounded-md border p-3"><div><Label className="cursor-pointer" htmlFor="smtp-enabled">启用邮件发送</Label><p className="mt-1 text-xs text-muted-foreground">保存后立即生效</p></div><Switch aria-label="启用邮件发送" checked={form.enabled} id="smtp-enabled" onCheckedChange={(checked) => setForm({ ...form, enabled: checked })} /></div><dl className="grid gap-3 text-sm"><div className="flex items-center justify-between gap-4"><dt className="text-muted-foreground">当前状态</dt><dd className="font-medium">{status}</dd></div><div className="flex items-center justify-between gap-4"><dt className="text-muted-foreground">凭据</dt><dd className="font-medium">{config.has_password ? "已保存" : "未保存"}</dd></div><div className="flex items-start justify-between gap-4"><dt className="text-muted-foreground">最近保存</dt><dd className="max-w-[190px] text-right font-medium">{formatDate(config.updated_at)}</dd></div></dl></CardContent>
          </Card>

          <Card>
            <CardHeader><CardTitle className="flex items-center gap-2"><Send className="size-4 text-muted-foreground" />发送测试邮件</CardTitle><CardDescription>使用最近保存并启用的配置验证连接和发件身份。</CardDescription></CardHeader>
            <CardContent className="space-y-3"><div className="space-y-2"><Label htmlFor="smtp-test-recipient">测试收件地址</Label><Input aria-describedby={testRecipientError ? "smtp-test-recipient-help smtp-test-recipient-error" : "smtp-test-recipient-help"} aria-invalid={testRecipientError ? true : undefined} autoComplete="email" id="smtp-test-recipient" onChange={(event) => { setTestRecipient(event.target.value); setTestRecipientError(""); }} placeholder="例如：admin@example.com" type="email" value={testRecipient} /><p className="text-xs text-muted-foreground" id="smtp-test-recipient-help">请输入要接收测试邮件的完整邮箱地址；内部地址可使用 admin@localhost。</p>{testRecipientError ? <p className="text-sm text-destructive" id="smtp-test-recipient-error" role="alert">{testRecipientError}</p> : null}</div><Button className="w-full" disabled={!form.enabled || !config.configured || testing || !testRecipient.trim()} onClick={() => void sendTest()} type="button" variant="outline"><Send />{testing ? "发送中..." : "发送测试邮件"}</Button>{!config.configured ? <p className="text-xs text-muted-foreground">先保存完整的 SMTP 配置后才能测试。</p> : !form.enabled ? <p className="text-xs text-muted-foreground">启用并保存邮件发送后才能测试。</p> : null}</CardContent>
          </Card>
        </div>
      </div>
    </form>
  );
}
