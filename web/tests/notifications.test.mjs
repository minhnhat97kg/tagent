import { test } from 'node:test'
import assert from 'node:assert/strict'
import { notifications, nextNotice } from '../src/lib/notifications.ts'
const agent = (pane,status,extra={}) => ({pane,status,name:pane,...extra})

test('notifications combine waiting and errored agents and blocked tickets without duplicates', () => {
 const agents=[agent('%1','waiting',{dialog:{question:'Approve?',options:[{Key:'1',Label:'Yes'}]}}),agent('%2','error'),agent('%3','done'),agent('%4','running'),agent('%5','running',{stuck:'8'}),agent('%6','idle')]
 const proj={tasks:[{id:'T1',status:'blocked',note:'Fallback'},{id:'T2',status:'todo'}],blockedReasons:{T1:'Review required'},workers:{T1:{role:'reviewer'}},statuses:[],roles:[]}
 const overview={attention:{items:[{kind:'quiet',pane:'%4'},{kind:'waiting',pane:'%1'},{kind:'blocked',count:1}]}}
 const items=notifications(agents,proj,overview)
 assert.deepEqual(items.map(i=>i.kind),['waiting','error','blocked'])
 assert.equal(items[0].agent.dialog.question,'Approve?')
 assert.equal(items.at(-1).reason,'Review required')
 assert.equal(items.at(-1).role,'reviewer')
 assert.equal(new Set(items.map(i=>i.id)).size,items.length)
})

test('n cycles the same notification list, tickets included', () => {
 const items=notifications([agent('%1','waiting'),agent('%2','done')],{tasks:[{id:'T1',status:'blocked'}],statuses:[]})
 assert.equal(items.length,2)
 assert.equal(nextNotice(items).id,'agent:%1')
 assert.equal(nextNotice(items,'agent:%1').id,'task:T1')
 assert.equal(nextNotice(items,'task:T1').id,'agent:%1')
 assert.equal(nextNotice([]),undefined)
})

test('blocked reply selects assignee or project role when no worker role exists', () => {
 const items=notifications([],{tasks:[{id:'T',status:'blocked',assignees:['builder']}],statuses:[],roles:['reviewer']})
 assert.equal(items[0].role,'builder')
 assert.deepEqual(notifications([],undefined,undefined),[])
})
