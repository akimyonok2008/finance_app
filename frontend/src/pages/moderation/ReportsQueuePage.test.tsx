import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const listReportsMock = vi.fn();
const getReportMock = vi.fn();
const resolveReportMock = vi.fn();

vi.mock("@/api/safety", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/safety")>();
  return {
    ...actual,
    listReports: (...args: unknown[]) => listReportsMock(...args),
    getReport: (...args: unknown[]) => getReportMock(...args),
    resolveReport: (...args: unknown[]) => resolveReportMock(...args),
  };
});

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/auth/useAuth", () => ({
  useAuth: () => ({ user: { id: "mod-1", role: "moderator" } }),
}));

vi.mock("@/components/layout/AppNav", () => ({ AppNav: () => null }));

// The status-filter <Select> includes an item with value="" (for "all"),
// which Radix's real Select component rejects at mount in this test
// environment. Stubbing it out keeps this test focused on the
// double-submit/no-raw-id behavior under test, unrelated to that filter.
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

const { ReportsQueuePage } = await import("@/pages/moderation/ReportsQueuePage");

const report = {
  id: "report-1",
  reporter_user_id: "u-reporter",
  reported_user_id: "u-target",
  category: "harassment",
  status: "open",
  created_at: new Date().toISOString(),
  explanation: "they were mean",
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ReportsQueuePage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listReportsMock.mockReset();
  getReportMock.mockReset();
  resolveReportMock.mockReset();
  listReportsMock.mockResolvedValue({ reports: [report] });
  getReportMock.mockResolvedValue(report);
});

describe("ReportsQueuePage status filter", () => {
  it("mounts the real Radix Select without an empty-string item value", async () => {
    vi.doUnmock("@/components/ui/select");
    vi.resetModules();
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { ReportsQueuePage: RealReportsQueuePage } = await import(
      "@/pages/moderation/ReportsQueuePage"
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <RealReportsQueuePage />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(screen.getByText("harassment")).toBeTruthy());

    expect(
      errorSpy.mock.calls.some((call) =>
        String(call[0]).includes("must have a value prop that is not an empty string"),
      ),
    ).toBe(false);
    errorSpy.mockRestore();
  });
});

describe("ReportsQueuePage moderation resolution", () => {
  it("disables the resolution buttons while a resolve request is pending, preventing a double-submit", async () => {
    let resolvePromise: (v: unknown) => void = () => {};
    resolveReportMock.mockReturnValue(
      new Promise((resolve) => {
        resolvePromise = resolve;
      }),
    );

    renderPage();
    await waitFor(() => expect(screen.getByText("harassment")).toBeTruthy());
    fireEvent.click(screen.getByText("harassment"));
    await waitFor(() => expect(screen.getByText("they were mean")).toBeTruthy());

    const dismissButton = screen.getByRole("button", { name: "Dismiss (no action)" });
    const applyButton = screen.getByRole("button", { name: "Apply action" });
    expect(dismissButton).toHaveProperty("disabled", false);

    fireEvent.click(dismissButton);
    await waitFor(() => expect(resolveReportMock).toHaveBeenCalledTimes(1));

    // Both buttons must now be disabled — a second click (on either button)
    // must not be able to fire a second resolve call while the first is
    // still in flight.
    await waitFor(() => {
      expect(dismissButton).toHaveProperty("disabled", true);
      expect(applyButton).toHaveProperty("disabled", true);
    });
    fireEvent.click(dismissButton);
    fireEvent.click(applyButton);
    expect(resolveReportMock).toHaveBeenCalledTimes(1);

    resolvePromise({});
  });

  it("never renders a raw subject id or any field beyond category/explanation/evidence for a report", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText("harassment")).toBeTruthy());
    fireEvent.click(screen.getByText("harassment"));
    await waitFor(() => expect(screen.getByText("they were mean")).toBeTruthy());

    expect(screen.queryByText("u-reporter")).toBeNull();
    expect(screen.queryByText("u-target")).toBeNull();
    expect(screen.queryByText(report.reporter_user_id)).toBeNull();
  });
});
