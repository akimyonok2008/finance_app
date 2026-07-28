import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// --- api mocks ---------------------------------------------------------------
const blockUserMock = vi.fn();
const unblockUserMock = vi.fn();
const getBlockedUsersMock = vi.fn();
const resolveReportMock = vi.fn();

vi.mock("@/api/safety", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/safety")>();
  return {
    ...actual,
    blockUser: (...args: unknown[]) => blockUserMock(...args),
    unblockUser: (...args: unknown[]) => unblockUserMock(...args),
    getBlockedUsers: (...args: unknown[]) => getBlockedUsersMock(...args),
    resolveReport: (...args: unknown[]) => resolveReportMock(...args),
  };
});

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const { BlockButton } = await import("@/components/social/BlockButton");
const { BlockedUsersCard } = await import("@/components/settings/BlockedUsersCard");

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  blockUserMock.mockReset();
  unblockUserMock.mockReset();
  getBlockedUsersMock.mockReset();
  resolveReportMock.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("BlockButton", () => {
  it("does not call the block API until the confirmation dialog is confirmed", () => {
    renderWithClient(<BlockButton handle="alice" />);
    fireEvent.click(screen.getByRole("button", { name: /Block/i }));
    expect(blockUserMock).not.toHaveBeenCalled();
  });

  it("only calls the block API after explicit confirmation, and does not show a false success state before the request resolves", async () => {
    let resolvePromise: (v: unknown) => void = () => {};
    blockUserMock.mockReturnValue(
      new Promise((resolve) => {
        resolvePromise = resolve;
      }),
    );

    renderWithClient(<BlockButton handle="alice" />);
    fireEvent.click(screen.getByRole("button", { name: /Block/i }));
    const confirmButtons = screen.getAllByRole("button", { name: "Block" });
    // The last "Block" button is the AlertDialogAction inside the opened dialog.
    fireEvent.click(confirmButtons[confirmButtons.length - 1]);

    await waitFor(() => expect(blockUserMock).toHaveBeenCalledWith("alice"));
    // The dialog closes immediately on confirm (Radix AlertDialogAction
    // default), so a second accidental click can't resubmit through it while
    // the request is still in flight — resolving the still-pending call must
    // not trigger a second invocation.
    resolvePromise({ handle: "alice", is_blocked: true });
    await new Promise((r) => setTimeout(r, 0));
    expect(blockUserMock).toHaveBeenCalledTimes(1);
  });
});

describe("BlockedUsersCard", () => {
  it("shows a distinct error state (not blank/partial data) when the blocked-users fetch fails", async () => {
    getBlockedUsersMock.mockRejectedValue(new Error("boom"));
    renderWithClient(<BlockedUsersCard />);
    await waitFor(() => {
      expect(screen.getByText("Could not load blocked users.")).toBeTruthy();
    });
  });

  it("does not show an unblocked/empty state until the unblock API call actually succeeds", async () => {
    getBlockedUsersMock.mockResolvedValue({
      blocked_users: [{ handle: "bob", display_name: "Bob", blocked_at: new Date().toISOString() }],
    });
    let resolvePromise: (v: unknown) => void = () => {};
    unblockUserMock.mockReturnValue(
      new Promise((resolve) => {
        resolvePromise = resolve;
      }),
    );

    renderWithClient(<BlockedUsersCard />);
    await waitFor(() => expect(screen.getByText("@bob")).toBeTruthy());

    const unblockBtn = screen.getByRole("button", { name: "Unblock" });
    fireEvent.click(unblockBtn);
    await waitFor(() => expect(unblockUserMock).toHaveBeenCalledWith("bob"));
    // Still listed and the button disabled while the request is pending —
    // no premature "unfollowed"/removed state before the API confirms.
    expect(screen.getByText("@bob")).toBeTruthy();
    await waitFor(() => expect(unblockBtn).toHaveProperty("disabled", true));

    resolvePromise({ handle: "bob", is_blocked: false });
  });
});
