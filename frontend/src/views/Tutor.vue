<script setup>
import { ref } from 'vue'
import { post, errorText } from '../api'

const title = ref('学习提问')
const content = ref('')
const sessionId = ref('')
const history = ref([])
const error = ref('')

async function ensure() {
  if (sessionId.value) return
  const res = await post('/aiChat/createSession', { title: title.value, lessonId: '' })
  sessionId.value = res.data.id
}

async function send() {
  error.value = ''
  try {
    await ensure()
    await post('/aiChat/sendMessage', { sessionId: sessionId.value, content: content.value })
    const rows = await post('/aiChat/history', { sessionId: sessionId.value })
    history.value = rows.data || []
    content.value = ''
  } catch (err) {
    error.value = errorText(err)
  }
}
</script>

<template>
  <div class="stack">
    <div>
      <h1 class="page-title">辅导</h1>
      <p class="lede">AI 关闭时，提问会返回“能力未启用”，课程和学习记录不受影响。</p>
    </div>
    <el-alert v-if="error" :title="error" type="warning" show-icon :closable="false" />
    <div class="card" v-for="msg in history" :key="msg.id">
      <span class="pill">{{ msg.role }}</span>
      <p>{{ msg.content || msg.status }}</p>
    </div>
    <div class="card stack">
      <el-input v-model="content" type="textarea" :rows="4" placeholder="想问的问题" />
      <el-button type="primary" @click="send">发送</el-button>
    </div>
  </div>
</template>
