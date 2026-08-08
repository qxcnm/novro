import RegisterClient from "./register-client";

type RegisterPageProps = {
  searchParams: Promise<{ ref?: string | string[] }>;
};

export default async function RegisterPage({ searchParams }: RegisterPageProps) {
  const params = await searchParams;
  const rawReferralCode = Array.isArray(params.ref) ? params.ref[0] : params.ref;
  const normalizedReferralCode = rawReferralCode?.trim().toUpperCase() ?? "";
  const referralCode = /^[A-Z0-9]{12}$/.test(normalizedReferralCode) ? normalizedReferralCode : "";

  return <RegisterClient initialReferralCode={referralCode} />;
}
