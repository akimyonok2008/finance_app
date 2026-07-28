import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const profileState = {
  isLoading: false,
  isError: false,
  data: {
    display_name: "Ada",
    bio: "",
    strategy_tag: "balanced_global",
    is_public: false,
    show_public_weights: false,
    public_preview: {},
  },
};

vi.mock("@/components/layout/AppNav", () => ({
  AppNav: () => <nav data-testid="nav" />,
}));
vi.mock("@/hooks/useProfile", () => ({
  useMyProfile: () => profileState,
  useUpdateProfile: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
}));
vi.mock("@/components/profile/ProfileForm", () => ({
  ProfileForm: () => <div data-testid="profile-settings">profile settings</div>,
}));
vi.mock("@/components/profile/ProfileSkeleton", () => ({
  ProfileSkeleton: () => <div>profile loading</div>,
}));
vi.mock("@/components/profile/PublicProfileDisplay", () => ({
  PublicProfileDisplay: () => <div data-testid="profile-preview">preview</div>,
}));
vi.mock("@/components/settings/PortfolioSettingsCard", () => ({
  PortfolioSettingsCard: () => <div data-testid="portfolio-settings">portfolio settings</div>,
}));
vi.mock("@/components/settings/ChangePasswordCard", () => ({
  ChangePasswordCard: () => <div data-testid="password-settings">password settings</div>,
}));
vi.mock("@/components/settings/DeleteAccountCard", () => ({
  DeleteAccountCard: () => <div data-testid="delete-settings">delete settings</div>,
}));
vi.mock("@/components/settings/BlockedUsersCard", () => ({
  BlockedUsersCard: () => <div data-testid="blocked-users-settings">blocked users</div>,
}));

const { AccountSettingsPage } = await import(
  "@/pages/Settings/AccountSettingsPage"
);
const { MyProfilePage } = await import("@/pages/Profile/MyProfilePage");

function renderPage(page: React.ReactNode) {
  return render(<MemoryRouter>{page}</MemoryRouter>);
}

beforeEach(() => {
  profileState.isLoading = false;
  profileState.isError = false;
  profileState.data.is_public = false;
});

describe("canonical settings location", () => {
  it("contains profile, portfolio, password, and account lifecycle settings in a responsive grid", () => {
    renderPage(<AccountSettingsPage />);

    expect(screen.getByTestId("profile-settings")).toBeTruthy();
    expect(screen.getByTestId("portfolio-settings")).toBeTruthy();
    expect(screen.getByTestId("password-settings")).toBeTruthy();
    expect(screen.getByTestId("delete-settings")).toBeTruthy();
    const profileColumn = screen
      .getByTestId("profile-settings")
      .closest(".lg\\:col-span-7");
    expect(profileColumn).toBeTruthy();
  });

  it("removes the duplicate settings mode from My Profile and links private-profile guidance to account settings", () => {
    renderPage(<MyProfilePage />);

    expect(screen.queryByRole("button", { name: "Settings" })).toBeNull();
    expect(screen.getByTestId("profile-preview")).toBeTruthy();
    const link = screen.getByRole("link", { name: "Account Settings" });
    expect(link.getAttribute("href")).toBe("/settings/account");
  });
});
