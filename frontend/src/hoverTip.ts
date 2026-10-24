import {reactive} from 'vue'

export const hoverTip = reactive({visible: false, text: '', x: 0, y: 0})

function place(event: MouseEvent) {
 const maxWidth = 340, maxHeight = 120, pad = 10
 hoverTip.x = Math.max(pad, Math.min(event.clientX + 14, window.innerWidth - maxWidth - pad))
 hoverTip.y = Math.max(pad, Math.min(event.clientY + 16, window.innerHeight - maxHeight - pad))
}

export function showHoverTip(event: MouseEvent, text: string) {
 hoverTip.text = text
 hoverTip.visible = true
 place(event)
}

export function hideHoverTip() {
 hoverTip.visible = false
}
