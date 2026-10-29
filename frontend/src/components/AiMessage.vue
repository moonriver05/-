<script setup lang="ts">
import { computed } from 'vue'
import { renderAIText } from '../aiText'
const props = defineProps<{ text: string }>()
const rendered = computed(() => renderAIText(props.text))
</script>
<template><div class="ai-message" v-html="rendered" /></template>
