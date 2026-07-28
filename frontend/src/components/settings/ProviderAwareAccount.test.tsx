import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

let hasPassword = true;
const deleteMutate = vi.fn();

vi.mock("@/auth/useAuth", () => ({
  useAuth: () => ({
    user: {
      id: "u1",
      email: "user@example.com",
      display_name: "User",
      email_verified: true,
      has_password: hasPassword,
    },
    logout: vi.fn(),
    replaceToken: vi.fn(),
  }),
}));

vi.mock("@/hooks/useAccount", () => ({
  useChangePassword: () => ({ mutate: vi.fn(), isPending: false }),
  useSetFirstPassword: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteAccount: () => ({ mutate: deleteMutate, isPending: false }),
}));

vi.mock("@/pages/auth/components/OAuthButtons", () => ({
  GoogleSignInButton: () => <button type="button">Confirm with Google</button>,
}));

const { ChangePasswordCard } = await import("@/components/settings/ChangePasswordCard");
const { DeleteAccountCard } = await import("@/components/settings/DeleteAccountCard");

beforeEach(() => {
  deleteMutate.mockReset();
  hasPassword = true;
});

describe("provider-aware account settings", () => {
  it("shows the current-password flow for password users", () => {
    render(<ChangePasswordCard />);
    expect(screen.getByText("Change password")).toBeTruthy();
    expect(screen.getByLabelText("Current password")).toBeTruthy();
  });

  it("does not ask provider-only users for an unknown password", () => {
    hasPassword = false;
    render(<ChangePasswordCard />);
    expect(screen.getByText("Create a password")).toBeTruthy();
    expect(screen.queryByLabelText("Current password")).toBeNull();
    expect(screen.getByRole("button", { name: "Confirm with Google" })).toBeTruthy();
  });

  it("requires password reauthentication before password-user deletion", () => {
    render(<MemoryRouter><DeleteAccountCard /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));
    expect(screen.getByLabelText("Password")).toBeTruthy();
  });

  it("requires linked-provider reauthentication before provider-user deletion", () => {
    hasPassword = false;
    render(<MemoryRouter><DeleteAccountCard /></MemoryRouter>);
    fireEvent.click(screen.getByRole("button", { name: "Delete account" }));
    expect(screen.queryByLabelText("Password")).toBeNull();
    expect(screen.getByRole("button", { name: "Confirm with Google" })).toBeTruthy();
  });
});
