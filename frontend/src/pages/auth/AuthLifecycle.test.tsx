import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const acceptSession = vi.fn();

vi.mock("@/auth/useAuth", () => ({
  useAuth: () => ({ acceptSession }),
}));

const { VerifyEmailPage } = await import("@/pages/auth/VerifyEmailPage");
const { ForgotPasswordPage } = await import("@/pages/auth/ForgotPasswordPage");
const { ResetPasswordPage } = await import("@/pages/auth/ResetPasswordPage");
const { apiRequest } = await import("@/api/client");
const { loginWithEmailRequest } = await import("@/api/authApi");
const { readStorage, writeStorage } = await import("@/auth/authStorage");

function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  }));
}

beforeEach(() => {
  acceptSession.mockReset();
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("authentication lifecycle pages", () => {
  it("verifies an email token and stores the resulting session", async () => {
    // The backend issues the session exclusively as an HttpOnly cookie —
    // the verify-email response body carries only `user`, never a token.
    const user = {
      id: "u1",
      email: "user@example.com",
      display_name: "User",
      email_verified: true,
      has_password: true,
    };
    vi.stubGlobal("fetch", vi.fn(() => jsonResponse({ user })));
    window.history.replaceState({}, "", "/verify-email?token=one-time-token&source=email#status");

    render(
      <MemoryRouter initialEntries={["/verify-email?token=one-time-token"]}>
        <VerifyEmailPage />
      </MemoryRouter>,
    );

    expect(window.location.pathname + window.location.search + window.location.hash)
      .toBe("/verify-email?source=email#status");
    expect(await screen.findByText("Email verified")).toBeTruthy();
    expect(acceptSession).toHaveBeenCalledWith({ token: "cookie-session", user });
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining("/auth/verify-email"),
      expect.objectContaining({
        body: JSON.stringify({ token: "one-time-token" }),
      }),
    );
  });

  it("uses the same forgot-password confirmation without exposing existence", async () => {
    vi.stubGlobal("fetch", vi.fn(() => jsonResponse({
      message: "If an account can be recovered, a reset email has been sent.",
    }, 202)));
    render(<MemoryRouter><ForgotPasswordPage /></MemoryRouter>);

    fireEvent.change(screen.getByLabelText("Email address"), {
      target: { value: "person@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send reset link" }));

    expect(await screen.findByText("If the account can be recovered, a reset email has been sent.")).toBeTruthy();
  });

  it("submits a one-time reset token and reports that older sessions were revoked", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(null, { status: 204 }))));
    render(
      <MemoryRouter initialEntries={["/reset-password?token=reset-once"]}>
        <ResetPasswordPage />
      </MemoryRouter>,
    );

    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "ReplacementPassword123" },
    });
    fireEvent.change(screen.getByLabelText("Confirm password"), {
      target: { value: "ReplacementPassword123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Update password" }));

    expect(await screen.findByText("Password updated. Older sessions have been revoked.")).toBeTruthy();
  });

  it("clears stored authentication after a revoked session receives 401", async () => {
    localStorage.setItem("finance_app_token", "revoked");
    localStorage.setItem("finance_app_user", JSON.stringify({ id: "u1" }));
    vi.stubGlobal("fetch", vi.fn(() => jsonResponse({ error: "invalid or expired token" }, 401)));

    await expect(apiRequest("/me")).rejects.toMatchObject({ status: 401 });
    await waitFor(() => {
      expect(localStorage.getItem("finance_app_token")).toBeNull();
      expect(localStorage.getItem("finance_app_user")).toBeNull();
    });
  });

  it("preserves moderator account state through login normalization and storage", async () => {
    const response = {
      token: "moderator-jwt",
      user: {
        id: "mod-1",
        email: "moderator@example.com",
        display_name: "Moderator",
        email_verified: true,
        has_password: true,
        role: "moderator",
        suspended_until: "2026-08-01T00:00:00Z",
        suspension_reason: "review",
      },
    };
    vi.stubGlobal("fetch", vi.fn(() => jsonResponse(response)));

    const session = await loginWithEmailRequest({
      email: response.user.email,
      password: "Password123",
    });
    expect(session.user).toMatchObject({
      role: "moderator",
      suspended_until: "2026-08-01T00:00:00Z",
      suspension_reason: "review",
    });

    writeStorage(session.token, session.user);
    expect(readStorage()).toEqual(session);
  });
});
