import { redirect } from "next/navigation";

export default function AdminLogsRedirect() {
  redirect("/admin/billing?tab=usage");
}
