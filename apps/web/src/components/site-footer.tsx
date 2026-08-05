import Link from "next/link";

import { Separator } from "@/components/ui/separator";

export function SiteFooter() {
  return (
    <footer className="border-t">
      <div className="mx-auto w-full max-w-7xl px-5 py-8 sm:px-8 lg:px-10">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-semibold">Novro Gateway</p>
            <p className="mt-1 text-sm text-muted-foreground">一个地址，接入主流国产模型。</p>
          </div>
          <nav className="flex flex-wrap items-center gap-4 text-sm text-muted-foreground" aria-label="页脚导航">
            <Link className="transition-colors hover:text-foreground" href="/models">模型与价格</Link>
            <Link className="transition-colors hover:text-foreground" href="/docs">接入文档</Link>
            <Link className="transition-colors hover:text-foreground" href="/login">控制台</Link>
          </nav>
        </div>
        <Separator className="my-6" />
        <p className="text-xs text-muted-foreground">连接团队、权限与主流国产模型。</p>
      </div>
    </footer>
  );
}
