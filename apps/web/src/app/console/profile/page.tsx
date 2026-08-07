"use client";

import { useState } from "react";
import { Check, Save } from "lucide-react";

import { useCurrentUser } from "@/components/console-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export default function ProfilePage() {
  const user = useCurrentUser();
  const [displayName, setDisplayName] = useState(user.display_name || user.username);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  async function save(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault(); setSaving(true); setMessage("");
    try {
      const response = await fetch("/api/account/profile", { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ display_name: displayName }) });
      if (!response.ok) { setMessage("资料保存失败，请稍后重试"); return; }
      setMessage("资料已更新");
    } catch { setMessage("资料保存失败，请稍后重试"); }
    finally { setSaving(false); }
  }
  return <div className="mx-auto max-w-2xl space-y-5"><div><p className="text-sm text-muted-foreground">用于识别账户和调用归属</p><h2 className="mt-1 text-2xl font-semibold tracking-tight">个人资料</h2></div><Card><CardHeader><CardTitle className="text-base">基本信息</CardTitle><CardDescription>用户名和邮箱由系统维护，显示名称可以随时修改。</CardDescription></CardHeader><CardContent><form className="space-y-5" onSubmit={save}><div className="space-y-2"><Label htmlFor="profile-username">用户名</Label><Input disabled id="profile-username" value={user.username} /></div><div className="space-y-2"><Label htmlFor="profile-email">邮箱</Label><Input disabled id="profile-email" value={user.email || "未设置邮箱"} /></div><div className="space-y-2"><Label htmlFor="profile-display-name">显示名称</Label><Input id="profile-display-name" maxLength={80} onChange={(event) => setDisplayName(event.target.value)} value={displayName} /></div><div className="flex flex-wrap items-center gap-3"><Badge variant="secondary">{user.role === "admin" ? "管理员" : "普通成员"}</Badge><Button disabled={saving || !displayName.trim()} type="submit"><Save />{saving ? "保存中..." : "保存资料"}</Button>{message ? <span className="flex items-center gap-1 text-sm text-emerald-700 dark:text-emerald-400" role="status"><Check className="size-4" />{message}</span> : null}</div></form></CardContent></Card></div>;
}
