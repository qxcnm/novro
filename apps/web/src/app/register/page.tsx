import RegisterClient from "./register-client";

type RegisterPageProps = {
  searchParams: Promise<{ ref?: string | string[] }>;
};

/**
 * RegisterPage 渲染对应的 React 界面组件。
 * @param searchParams 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default async function RegisterPage({ searchParams }: RegisterPageProps) {
  const params = await searchParams;
  const rawReferralCode = Array.isArray(params.ref) ? params.ref[0] : params.ref;
  const normalizedReferralCode = rawReferralCode?.trim().toUpperCase() ?? "";
  const referralCode = /^[A-Z0-9]{12}$/.test(normalizedReferralCode) ? normalizedReferralCode : "";

  return <RegisterClient initialReferralCode={referralCode} />;
}
