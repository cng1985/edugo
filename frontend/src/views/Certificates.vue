<script setup>
import { onMounted, ref } from 'vue'
import { post, errorText } from '../api'

const rows = ref([])
const courseId = ref('')
const error = ref('')
const message = ref('')

async function load() {
  const res = await post('/certificate/list', {})
  rows.value = res.data || []
}

async function issue() {
  error.value = ''
  try {
    const res = await post('/certificate/issue', { courseId: courseId.value })
    message.value = `证书号 ${res.data.certificateNo}`
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

onMounted(async () => {
  try { await load() } catch (err) { error.value = errorText(err) }
})
</script>

<template>
  <div class="stack">
    <div>
      <h1 class="page-title">证书</h1>
      <p class="lede">完成课程全部必修后可以申请。同一资格重复申请返回同一张证书。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <el-alert v-if="message" :title="message" type="success" show-icon :closable="false" />
    <div class="card row">
      <el-input class="grow" v-model="courseId" placeholder="课程 ID" />
      <el-button type="primary" @click="issue">申请</el-button>
    </div>
    <div class="card" v-for="row in rows" :key="row.id">
      <strong>{{ row.title }}</strong>
      <div class="muted">{{ row.certificateNo }} · {{ row.status }}</div>
    </div>
    <router-link to="/verify">公开核验页</router-link>
  </div>
</template>
