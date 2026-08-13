"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { Bell, CheckCircle2, Eye, RefreshCw, Save } from "lucide-react";
import { useRouter } from "next/navigation";

import { AnnouncementDialog } from "@/components/announcement-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { emptyAnnouncement, type Announcement, readAnnouncementError } from "@/lib/announcement";

type AnnouncementConfig = Omit<Announcement, "available"> & {
  enabled: boolean;
  updated_at?: string;
};

const emptyConfig: AnnouncementConfig = { enabled: false, title: "", body: "" };

function normalizeConfig(value?: Partial<AnnouncementConfig>): AnnouncementConfig {
  return {
    enabled: value?.enabled === true,
    title: typeof value?.title === "string" ? value.title : "",
    body: typeof value?.body === "string" ? value.body : "",
    updated_at: value?.updated_at,
  };
}

function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "尚未保存";
}

export default function AnnouncementSettingsClient() {
  const router = useRouter();
  const [config, setConfig] = useState<AnnouncementConfig>(emptyConfig);
  const [form, setForm] = useState<AnnouncementConfig>(emptyConfig);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [previewOpen, setPreviewOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/admin/announcement", { cache: "no-store", credentials: "same-origin" });
      if (response.status === 401) { window.location.replace("/login"); return; }
      if (response.status === 403) { router.replace("/console"); return; }
      if (!response.ok) { setError(await readAnnouncementError(response)); return; }
      const next = normalizeConfig(((await response.json()) as { announcement?: Partial<AnnouncementConfig> }).announcement);
      setConfig(next);
      setForm(next);
    } catch {
      setError("系统公告加载失败，请稍后重试");
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
    const title = form.title.trim();
    const body = form.body.trim();
    if (form.enabled && (!title || !body)) {
      setError("启用公告前必须填写标题和正文");
      return;
    }
    setSaving(true);
    try {
      const response = await fetch("/api/admin/announcement", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ enabled: form.enabled, title, body }),
      });
      if (!response.ok) { setError(await readAnnouncementError(response)); return; }
      const next = normalizeConfig(((await response.json()) as { announcement?: Partial<AnnouncementConfig> }).announcement);
      setConfig(next);
      setForm(next);
      setMessage(next.enabled ? "系统公告已发布，用户下次打开控制台时会自动看到。" : "系统公告已保存为未启用状态，用户端不会展示草稿内容。");
    } catch {
      setError("系统公告保存失败，请稍后重试");
    } finally {
      setSaving(false);
    }
  }

  const preview: Announcement = form.title.trim() && form.body.trim()
    ? { available: true, title: form.title.trim(), body: form.body.trim() }
    : emptyAnnouncement;

  return (
    <>
      <form className="mx-auto flex max-w-4xl flex-col gap-5" onSubmit={save}>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground"><Bell aria-hidden="true" className="size-4" /></span>
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2"><p className="font-medium">系统公告</p><Badge variant={config.enabled ? "default" : "secondary"}>{config.enabled ? "展示中" : "未启用"}</Badge></div>
              <p className="text-sm text-muted-foreground">维护用户进入控制台时自动展示的平台通知。</p>
            </div>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button aria-label="预览系统公告" disabled={loading || !preview.available} onClick={() => setPreviewOpen(true)} title="预览系统公告" type="button" variant="outline"><Eye data-icon="inline-start" />预览</Button>
            <Button aria-label="刷新系统公告" disabled={loading || saving} onClick={() => void load()} size="icon" title="刷新系统公告" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
            <Button disabled={loading || saving} type="submit"><Save data-icon="inline-start" />{saving ? "保存中..." : "保存公告"}</Button>
          </div>
        </div>

        {message ? <Alert><CheckCircle2 /><AlertDescription>{message}</AlertDescription></Alert> : null}
        {error ? <Alert variant="destructive"><Bell /><AlertDescription>{error}</AlertDescription></Alert> : null}

        <Card>
          <CardHeader>
            <CardTitle>公告内容</CardTitle>
            <CardDescription>支持纯文本和换行，不渲染 HTML；停用后仍会保留草稿。</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="announcement-title">公告标题</FieldLabel>
                <Input disabled={loading || saving} id="announcement-title" maxLength={120} onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))} placeholder="例如：服务维护通知" value={form.title} />
                <FieldDescription>{form.title.length}/120 个字符</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="announcement-body">公告正文</FieldLabel>
                <Textarea className="min-h-64 resize-y" disabled={loading || saving} id="announcement-body" maxLength={10_000} onChange={(event) => setForm((current) => ({ ...current, body: event.target.value }))} placeholder="填写公告内容，可使用换行组织信息。" value={form.body} />
                <FieldDescription>{form.body.length}/10000 个字符</FieldDescription>
              </Field>
            </FieldGroup>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>展示状态</CardTitle><CardDescription>启用后，已登录用户每次重新打开控制台都会看到公告，并可随时通过页头铃铛重新查看。</CardDescription></CardHeader>
          <CardContent className="flex flex-col gap-5">
            <Field className="rounded-lg border p-4" orientation="horizontal">
              <FieldContent>
                <FieldLabel className="cursor-pointer" htmlFor="announcement-enabled">启用系统公告</FieldLabel>
                <FieldDescription>标题和正文完整时才可启用。</FieldDescription>
              </FieldContent>
              <Switch aria-label="启用系统公告" checked={form.enabled} disabled={loading || saving} id="announcement-enabled" onCheckedChange={(enabled) => setForm((current) => ({ ...current, enabled }))} />
            </Field>
            <dl className="grid gap-4 border-t pt-5 text-sm sm:grid-cols-2">
              <div><dt className="text-muted-foreground">当前状态</dt><dd className="mt-1 font-medium">{config.enabled ? "用户端展示中" : "用户端不展示"}</dd></div>
              <div><dt className="text-muted-foreground">最近保存</dt><dd className="mt-1 font-medium">{formatDate(config.updated_at)}</dd></div>
            </dl>
          </CardContent>
        </Card>
      </form>

      <AnnouncementDialog announcement={preview} error="" loading={false} onDismissForToday={() => setPreviewOpen(false)} onOpenChange={setPreviewOpen} onRetry={() => undefined} open={previewOpen} />
    </>
  );
}
