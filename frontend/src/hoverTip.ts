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

// Saved reasons can be a whole error chain or a generated round summary; the tip
// only needs the cause, so drop the leading probe URL and clamp both ends.
export function shortReason(text: string) {
 const at = text.lastIndexOf('connect error: ')
 const tail = (at >= 0 ? text.slice(at + 'connect error: '.length) : text).replace(/\s+/g, ' ').trim()
 return tail.length > 72 ? tail.slice(0, 44) + ' … ' + tail.slice(-24) : tail
}
