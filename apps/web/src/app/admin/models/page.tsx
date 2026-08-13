import { redirect } from "next/navigation";

/**
 * AdminModelsPage 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function AdminModelsPage() {
  redirect("/admin/providers");
}
