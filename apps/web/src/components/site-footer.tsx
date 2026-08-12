import Link from "next/link";
import { connection } from "next/server";

import { Separator } from "@/components/ui/separator";

export async function SiteFooter() {
  await connection();
  const filingNumber = process.env.NOVRO_FILING_NUMBER?.trim();
  const year = new Date().getFullYear();

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
        <div className="flex flex-col gap-2 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          {filingNumber ? (
            <a
              className="transition-colors hover:text-foreground"
              href="https://beian.miit.gov.cn/"
              rel="noreferrer"
              target="_blank"
            >
              {filingNumber}
            </a>
          ) : null}
          <p className={filingNumber ? undefined : "sm:ml-auto"}>
            © {year}{" "}
            <Link className="font-semibold text-foreground transition-colors hover:text-primary" href="/">
              Novro
            </Link>
            . 版权所有，由 Novro 项目贡献者设计与开发。
          </p>
        </div>
      </div>
    </footer>
  );
}
