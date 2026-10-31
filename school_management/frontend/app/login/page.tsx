import { redirect } from "next/navigation";
import LoginForm from "@/components/LoginForm";
import Brand from "@/components/Brand";
import Link from "next/link";
import { currentDestination } from "@/lib/session";

const MESSAGES: Record<string, string> = {
  auth: "Login could not be completed. Please try again.",
  server: "You are logged in, but the server could not be reached. Please try again in a moment.",
};

export default async function LoginPage({ searchParams }: { searchParams: Promise<{ error?: string }> }) {
  const { error } = await searchParams;
  const destination = error === "server" ? null : await currentDestination();
  if (destination) redirect(destination);

  return (
    <div className="auth-page">
      <aside className="auth-story">
        <Link href="/" aria-label="Campus One home"><Brand light /></Link>
        <div className="auth-story-copy">
          <span className="eyebrow">Your school, in sync</span>
          <h2>Good things happen when everything has its place.</h2>
          <p>A thoughtful workspace for the people who make every school day count.</p>
          <div className="auth-story-rule" />
          <div className="auth-story-points"><span>01 <b>Organize classes</b></span><span>02 <b>Plan each day</b></span><span>03 <b>See progress</b></span></div>
        </div>
        <span className="auth-story-footer">One connected school workspace</span>
      </aside>
      <main className="auth-main">
        <div className="auth-mobile-brand"><Link href="/" aria-label="Campus One home"><Brand /></Link></div>
        <div className="auth-content">
          <span className="eyebrow">Welcome to Campus One</span>
          <h1>Welcome back<span className="auth-heading-dot">.</span></h1>
          <p className="auth-subtitle">Sign in to pick up right where your school day left off.</p>
          <LoginForm initialError={error ? (MESSAGES[error] ?? MESSAGES.auth) : null} />
          <p className="auth-footnote">Secure access for your school community</p>
        </div>
      </main>
    </div>
  );
}
