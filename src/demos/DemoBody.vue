<script setup>
import { computed } from 'vue'
import BasicDemo from './BasicDemo.vue'
import ListDemo from './ListDemo.vue'
import GestureDemo from './GestureDemo.vue'
import DataDemo from './DataDemo.vue'
import AmbientDemo from './AmbientDemo.vue'
import NavDemo from './NavDemo.vue'

const props = defineProps({
  kind: String, body: String, cls: String, no: Number, tick: Number,
})

const basic = ['css', 'skeleton', 'press', 'like', 'check', 'switch', 'checkbox', 'focus', 'shake', 'toast', 'odo', 'pulse', 'liquid']
const list = ['stagger', 'flip', 'insert', 'collapse', 'magnet', 'peek', 'accordion', 'masonry']
const gesture = ['drawer', 'pull', 'swipe', 'longpress', 'pinch', 'reorder', 'sheet', 'bounce']
const data = ['ring', 'line', 'bars', 'area', 'heat', 'radar', 'spark']
const ambient = ['spot', 'noise', 'sheen', 'caret', 'breathe', 'ripple', 'particles', 'aurora']
const nav = ['ink', 'crumb', 'burger', 'hidebar', 'anchor', 'progress']

const comp = computed(() => {
  if (basic.includes(props.kind)) return BasicDemo
  if (list.includes(props.kind)) return ListDemo
  if (gesture.includes(props.kind)) return GestureDemo
  if (data.includes(props.kind)) return DataDemo
  if (ambient.includes(props.kind)) return AmbientDemo
  if (nav.includes(props.kind)) return NavDemo
  return BasicDemo
})
</script>

<template>
  <component :is="comp" :kind="kind" :body="body" :cls="cls" :no="no" :tick="tick" />
</template>
