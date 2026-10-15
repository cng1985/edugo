<script setup>
import { onMounted, ref } from 'vue'
import { post, errorText } from '../api'
import { useSession } from '../session'

const session = useSession()
const stats = ref({ due: 0, courses: 0, notices: 0 })
const error = ref('')

onMounted(async () => {
  try {
    const [review, enroll, notes] = await Promise.all([
      post('/reviewCard/stats', {}),
      post('/enrollment/list', {}),
      post('/notification/list', {}),
    ])
    stats.value = {
      due: review.data?.due || 0,
      courses: Array.isArray(enroll.data) ? enroll.data.length : 0,
      notices: Array.isArray(notes.data) ? notes.data.filter((n) => !n.readAt).length : 0,
    }
  } catch (err) {
    error.value = errorText(err)
  }
})
</script>

<template>
  <div>
    <h1 class="page-title">你好，{{ session.user?.displayName }}</h1>
    <p class="lede">从一门已发布课程开始，完成必修课时后可以申请证书。</p>
    <el-alert v-if="error" :title="error" type="warning" show-icon :closable="false" />
    <div class="grid">
      <div class="card"><div class="muted">待复习</div><div class="stat">{{ stats.due }}</div></div>
      <div class="card"><div class="muted">在学课程</div><div class="stat">{{ stats.courses }}</div></div>
      <div class="card"><div class="muted">未读通知</div><div class="stat">{{ stats.notices }}</div></div>
    </div>
  </div>
</template>
