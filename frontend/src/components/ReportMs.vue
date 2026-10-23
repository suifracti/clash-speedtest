<script setup lang="ts">
import {computed} from 'vue'
const props=defineProps<{value:number|null|undefined}>()
const rounded=computed(()=>props.value===null||props.value===undefined||!Number.isFinite(props.value)?null:Math.round(props.value))
</script>
<template>
 <span class="report-ms"><template v-if="rounded===null">—</template><template v-else>{{rounded}}<span class="report-ms-unit">ms</span></template></span>
</template>
<style scoped>
.report-ms{font-variant-numeric:tabular-nums;white-space:nowrap}
.report-ms-unit{margin-left:4px}
</style>
