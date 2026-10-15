<script setup>
import { onMounted, ref } from 'vue'
import { post, errorText } from '../api'

const suggestions = ref([])
const reports = ref([])
const error = ref('')

async function load() {
  const [s, r] = await Promise.all([post('/aiSuggestion/list', {}), post('/learningReport/list', {})])
  suggestions.value = s.data || []
  reports.value = r.data || []
}

async function suggest() {
  error.value = ''
  try {
    await post('/aiSuggestion/generate', {})
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

async function report() {
  await post('/learningReport/create', {})
  await load()
}

onMounted(async () => {
  try { await load() } catch (err) { error.value = errorText(err) }
})
</script>

<template>
  <div class="stack">
    <div>
      <h1 class="page-title">报告</h1>
      <p class="lede">建议只读取投影，不会改写计划、成绩或证书。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="row">
      <el-button type="primary" @click="suggest">生成建议</el-button>
      <el-button @click="report">生成报告</el-button>
    </div>
    <div class="card" v-for="item in suggestions" :key="item.id">{{ item.content }}</div>
    <div class="card" v-for="item in reports" :key="item.id">
      <strong>{{ item.title }}</strong>
      <p>{{ item.body }}</p>
    </div>
  </div>
</template>
