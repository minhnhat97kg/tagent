import { act, cleanup, renderHook } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { usePoll } from "./usePoll"

afterEach(cleanup)

test("a slow response older than the last applied one is dropped", async () => {
  const pending: ((v: string) => void)[] = []
  const fn = vi.fn(() => new Promise<string>(resolve => { pending.push(resolve) }))
  const { result } = renderHook(() => usePoll(fn, 60_000, "k"))
  // pending[0] is the mount load; reload() sends a newer request.
  act(() => { void result.current.reload() })
  await act(async () => { pending[1]("new") })
  expect(result.current.data).toBe("new")
  await act(async () => { pending[0]("old") })
  expect(result.current.data).toBe("new")
})
