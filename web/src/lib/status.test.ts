import { expect, test } from "vitest"
import { shortPrompt } from "./status"

test("shortPrompt shows the message, not pasted-content markup", () => {
  expect(shortPrompt('<pasted_content id="d27">\nTicket 61: UI/UX review')).toBe("Ticket 61: UI/UX review")
  expect(shortPrompt("<system-reminder>\nnoise\n</system-reminder>\nfix the build")).toBe("fix the build")
  expect(shortPrompt("plain first line\nsecond")).toBe("plain first line")
})
