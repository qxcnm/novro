import type { Metadata } from "next";
import { Clock3 } from "lucide-react";

import { SiteFooter } from "@/components/site-footer";
import { SiteHeader } from "@/components/site-header";
import { ModelsClient } from "@/app/models/models-client";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { PRICE_VERIFIED_AT, modelCatalog } from "@/lib/models";

export const metadata: Metadata = {
  title: "模型与官方价格",
  description: "查看 Novro 汇集的 Kimi、GLM、DeepSeek 模型、能力参数与厂商官方牌价。",
};

export default function ModelsPage() {
  return (
    <div className="min-h-screen bg-muted/30">
      <SiteHeader />
      <main>
        <section className="border-b bg-background">
          <div className="mx-auto w-full max-w-7xl px-5 py-10 sm:px-8 sm:py-12 lg:px-10">
            <p className="text-sm font-medium text-muted-foreground">模型目录</p>
            <h1 className="mt-3 text-4xl font-semibold sm:text-5xl">模型与官方牌价</h1>
            <p className="mt-5 max-w-3xl text-base leading-7 text-muted-foreground sm:text-lg sm:leading-8">
              共 {modelCatalog.length} 款目录项。价格为模型厂商公开的按量牌价，单位人民币 / 百万 tokens；实际 Novro 结算价以管理员模型路由配置为准。
            </p>
            <p className="mt-3 text-sm text-muted-foreground">
              官方价格核验日期：{PRICE_VERIFIED_AT}。厂商可能随时调整价格，结算前请复核卡片中的官方来源。
            </p>
            <Alert className="mt-5">
              <Clock3 aria-hidden="true" />
              <AlertTitle>DeepSeek 峰谷定价尚未生效</AlertTitle>
              <AlertDescription>
                DeepSeek 官方说明将来会采用峰谷定价，高峰时段为北京时间每日 09:00-12:00 和 14:00-18:00，
                届时所有计费项为平时价格的 2 倍，具体生效时间以官方通知为准。下方卡片展示当前公开价。
              </AlertDescription>
            </Alert>
          </div>
        </section>
        <ModelsClient initialModels={modelCatalog} />
      </main>
      <SiteFooter />
    </div>
  );
}
