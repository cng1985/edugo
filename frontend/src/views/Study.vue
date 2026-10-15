<script setup>
import { onMounted, ref } from 'vue'
import { post, errorText } from '../api'

const courses = ref([])
const enrollments = ref([])
const active = ref(null)
const progress = ref(null)
const message = ref('')
const error = ref('')

async function load() {
  const [listed, joined] = await Promise.all([
    post('/course/list', { page: 1, size: 50, status: 'published' }),
    post('/enrollment/list', {}),
  ])
  courses.value = listed.data || []
  enrollments.value = joined.data || []
}

async function join(course) {
  error.value = ''
  try {
    await post('/enrollment/join', { courseId: course.id })
    message.value = `已加入 ${course.title}`
    await load()
    await showProgress(course.id)
  } catch (err) {
    error.value = errorText(err)
  }
}

async function showProgress(courseId) {
  const detail = await post('/course/detail', { courseId })
  active.value = detail.data
  const row = await post('/progress/get', { courseId })
  progress.value = row.data
}

async function confirmLesson(lesson) {
  error.value = ''
  try {
    const session = await post('/studyRecord/startSession', { lessonId: lesson.id })
    await post('/studyRecord/submit', { sessionId: session.data.id, sequence: 1, recordType: lesson.lessonType === 'exercise' ? 'exercise' : 'confirm', passed: true })
    message.value = `已记录：${lesson.title}`
    await showProgress(active.value.id)
  } catch (err) {
    error.value = errorText(err)
  }
}

onMounted(async () => {
  try {
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
})
</script>

<template>
  <div class="stack">
    <div>
      <h1 class="page-title">学习</h1>
      <p class="lede">加入已发布课程，确认文档或完成练习后更新进度。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <el-alert v-if="message" :title="message" type="success" show-icon :closable="false" />
    <div class="grid">
      <div class="card stack" v-for="course in courses" :key="course.id">
        <h3>{{ course.title }}</h3>
        <p class="muted">{{ course.summary }}</p>
        <div class="row">
          <el-button type="primary" @click="join(course)">加入</el-button>
          <el-button @click="showProgress(course.id)">进度</el-button>
        </div>
      </div>
    </div>
    <div class="card" v-if="progress">
      <h3>{{ active?.title }}</h3>
      <p>完成 {{ progress.requiredDone }} / {{ progress.requiredTotal }} 门必修，进度 {{ progress.rate ?? '—' }}%</p>
      <p v-if="progress.completed">课程已完成，可以前往证书页申请。</p>
      <div v-for="chapter in active?.chapters || []" :key="chapter.id">
        <strong>{{ chapter.title }}</strong>
        <div class="row" v-for="lesson in chapter.lessons || []" :key="lesson.id">
          <span>{{ lesson.title }}</span>
          <el-button size="small" @click="confirmLesson(lesson)">完成本课</el-button>
        </div>
      </div>
    </div>
    <p class="muted" v-if="enrollments.length">已加入 {{ enrollments.length }} 门课程。</p>
  </div>
</template>
