import { afterEach, describe, expect, it, vi } from "vitest";

import { depositCash, withdrawCash, createPosition } from "@/api/portfolioApi";

/**
 * Verifies decimal-typed mutation request bodies are sent as canonical
 * strings, matching the backend's money.* decoder, never as raw JS numbers.
 */
describe("mutation request bodies send decimal strings", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function stubFetch(): { body: () => unknown } {
    let captured: unknown;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        captured = init?.body ? JSON.parse(init.body as string) : undefined;
        return new Response(JSON.stringify({}), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    return { body: () => captured };
  }

  it("deposit sends amount as a string", async () => {
    const fetchStub = stubFetch();
    await depositCash({ currency: "USD", amount: "150.50" });
    expect(fetchStub.body()).toMatchObject({ currency: "USD", amount: "150.50" });
    expect(typeof (fetchStub.body() as { amount: unknown }).amount).toBe("string");
  });

  it("withdrawal sends amount as a string", async () => {
    const fetchStub = stubFetch();
    await withdrawCash({ currency: "USD", amount: "42.00" });
    expect(typeof (fetchStub.body() as { amount: unknown }).amount).toBe("string");
  });

  it("buy sends quantity/execution_price/fee as strings", async () => {
    let captured: unknown;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        captured = init?.body ? JSON.parse(init.body as string) : undefined;
        return new Response(
          JSON.stringify({ position: { id: "p1" }, portfolio_version: 1, ranked_index: "100", ranking_status: "active" }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    await createPosition({
      symbol: "AAPL",
      asset_type: "stock",
      quantity: "10",
      execution_price: "189.42",
      fee: "1.00",
    });
    const body = captured as Record<string, unknown>;
    expect(typeof body.quantity).toBe("string");
    expect(typeof body.execution_price).toBe("string");
    expect(typeof body.fee).toBe("string");
  });
});
