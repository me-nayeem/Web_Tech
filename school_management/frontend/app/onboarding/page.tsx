import { redirect } from "next/navigation";
import StudentOnboardingForm from "@/components/StudentOnboardingForm";
import TeacherOnboardingForm from "@/components/TeacherOnboardingForm";
import { fetchList } from "@/lib/api";
import { destinationFor } from "@/lib/routes";
import { requireUser } from "@/lib/session";

export default async function OnboardingPage() {
  const me = await requireUser();
  const destination = destinationFor(me);
  if (destination !== "/onboarding") redirect(destination);

  if (me.role === "TEACHER") {
    if (!me.onboardingCompleted) {
      return (
        <main className="narrow">
          <div className="message error" role="alert">
            Your account cannot be set up yet. Please ask an administrator to check your role.
          </div>
        </main>
      );
    }
    const subjects = await fetchList("/api/subjects");
    return (
      <main className="narrow">
        <h1>Teacher details</h1>
        <p className="muted">Tell us about your qualification and the subjects you can teach. You only do this once.</p>
        <TeacherOnboardingForm subjects={subjects} />
      </main>
    );
  }

  const classes = await fetchList("/api/classes");
  return (
    <main className="narrow">
      <h1>Welcome</h1>
      <p className="muted">Complete your details to continue. You only do this once.</p>
      <StudentOnboardingForm classes={classes} />
    </main>
  );
}