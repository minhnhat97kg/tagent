import { beforeEach, afterEach, expect, test, vi } from "vitest"
import { cleanup, render, screen, fireEvent, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { ChatPanel } from "./ChatPanel"
import type { Chat } from "@/types"
const mocks=vi.hoisted(() => ({post:vi.fn(),reload:vi.fn(),chat:{} as Chat}))
vi.mock("@/api",()=>({api:vi.fn(),post:mocks.post}))
vi.mock("@/hooks/usePoll",()=>({usePoll:()=>({data:mocks.chat,reload:mocks.reload})}))
beforeEach(()=>{
 mocks.chat={dir:"/repo",enabled:true,revision:1,status:"idle",messages:[],response:{question:"",answer:"",escalate:"",draft:null},workflows:[],actions:[],recipients:[{Name:"builder",Pane:"%1",Role:"build"}]}
 mocks.post.mockReset().mockResolvedValue({});mocks.reload.mockReset()
 Element.prototype.scrollTo=vi.fn()
})
afterEach(cleanup)
function mount(){render(<ChatPanel dir="/repo" onTasks={()=>{}}/>);return screen.getByRole("textbox",{name:"Chat message"}) as HTMLTextAreaElement}
test("Enter sends, Shift+Enter inserts newline, and IME does not send",async()=>{
 const input=mount(),user=userEvent.setup()
 await user.type(input,"hello{Shift>}{Enter}{/Shift}world")
 expect(input.value).toBe("hello\nworld")
 fireEvent.keyDown(input,{key:"Enter",isComposing:true});expect(mocks.post).not.toHaveBeenCalled()
 await user.keyboard("{Enter}")
 expect(mocks.post).toHaveBeenCalledWith("/api/chat",expect.objectContaining({op:"send",text:"hello\nworld"}))
 await waitFor(()=>expect(input.value).toBe(""))
})
test("failed sends preserve input",async()=>{
 mocks.post.mockRejectedValue(new Error("offline"))
 const input=mount();await userEvent.type(input,"keep this{Enter}")
 await screen.findByRole("alert");expect(input.value).toBe("keep this")
})
test("mentions accept selection before sending",async()=>{
 const input=mount();await userEvent.type(input,"@b")
 expect(screen.getByRole("listbox")).toBeTruthy()
 await userEvent.keyboard("{ArrowDown}{Enter}")
 expect(input.value).toBe("@build ");expect(mocks.post).not.toHaveBeenCalled()
 await userEvent.type(input,"do tests{Enter}")
 expect(mocks.post).toHaveBeenCalledWith("/api/chat",expect.objectContaining({text:"@build do tests"}))
})
test("new chat is usable while the model works",async()=>{
 mocks.chat.status="running";mount()
 await userEvent.click(screen.getByRole("button",{name:"New chat"}))
 expect(mocks.post).toHaveBeenCalledWith("/api/chat",expect.objectContaining({op:"clear",revision:1}))
})
test("tool cards show routing, arguments, result and explicit approvals",async()=>{
 mocks.chat.status="proposal";mocks.chat.actions=["approve","cancel"]
 mocks.chat.messages=[{role:"assistant",text:"",revision:1,tool:{Calls:[{name:"send_prompt",args:{pane:"%1",prompt:"test"}}],Status:"proposed",Output:"queued"}}]
 mount();expect(screen.getByText("send_prompt → %1")).toBeTruthy();expect(screen.getByText("queued")).toBeTruthy()
 expect(mocks.post).not.toHaveBeenCalled()
 await userEvent.click(screen.getByRole("button",{name:"approve"}))
 expect(mocks.post).toHaveBeenCalledWith("/api/chat",expect.objectContaining({op:"approve"}))
})
test("markdown cannot inject HTML or unsafe links",()=>{
 mocks.chat.messages=[{role:"assistant",text:'**Hello** <img src=x onerror="alert(1)"> [bad](javascript:alert(1))',revision:1}]
 mount();expect(screen.getByText("Hello").tagName).toBe("STRONG")
 expect(document.querySelector("img")).toBeNull()
 expect(document.querySelector('a[href^="javascript:"]')).toBeNull()
})
