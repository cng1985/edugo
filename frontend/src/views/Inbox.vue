<script setup>
import { onMounted, ref } from 'vue'
import { post, errorText } from '../api'

const rows = ref([])
const prefs = ref(null)
const error = ref('')

async function load() {
  const [list, preference] = await Promise.all([post('/notification/list', {}), post('/notification/preferences', {})])
  rows.value = list.data || []
  prefs.value = preference.data
}

async function read(row) {
  await post('/notification/read', { id: row.id })
  await load()
}

async function savePrefs() {
  await post('/notification/updatePreferences', prefs.value)
}

onMounted(async () => {
  try { await load() } catch (err) { error.value = errorText(err) }
})
</script>

<template>
  <div class="stack">
    <h1 class="page-title">通知</h1>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card stack" v-if="prefs">
      <el-checkbox v-model="prefs.review">复习提醒</el-checkbox>
      <el-checkbox v-model="prefs.plan">计划提醒</el-checkbox>
      <el-checkbox v-model="prefs.score">成绩提醒</el-checkbox>
      <el-checkbox v-model="prefs.cert">证书提醒</el-checkbox>
      <el-button @click="savePrefs">保存偏好</el-button>
    </div>
    <div class="card row" v-for="row in rows" :key="row.id">
      <div class="grow">
        <strong>{{ row.title }}</strong>
        <div class="muted">{{ row.body }}</div>
      </div>
      <el-button v-if="!row.readAt" @click="read(row)">标为已读</el-button>
      <span v-else class="pill">已读</span>
    </div>
    <p class="muted" v-if="!rows.length">还没有通知。</p>
  </div>
</template>
