<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'
import { useSession } from '../session'

const session = useSession()
const exams = ref([])
const questions = ref([])
const attempt = ref(null)
const choices = ref({})
const error = ref('')
const message = ref('')
const form = reactive({ title: '随堂测验', questionId: '', pass: 60 })

async function load() {
  const res = await post('/exam/list', {})
  exams.value = res.data || []
  if (session.canTeach) {
    const bank = await post('/question/list', { page: 1, size: 50, status: 'approved' })
    questions.value = bank.data || []
  }
}

async function publishExam() {
  error.value = ''
  try {
    const picked = questions.value.find((q) => q.id === form.questionId)
    if (!picked) throw new Error('请选择已通过的题目')
    const paper = await post('/paper/create', { title: form.title })
    const updated = await post('/paper/update', {
      paperId: paper.data.id,
      title: form.title,
      expectedVersion: paper.data.version,
      questionVersionIds: [picked.versionId],
      scores: [1000],
    })
    await post('/paper/publish', { paperId: paper.data.id, expectedVersion: updated.data.version })
    const exam = await post('/exam/create', {
      title: form.title,
      paperId: paper.data.id,
      durationSec: 1800,
      maxAttempts: 2,
      passScoreCenti: Math.round(Number(form.pass) * 100),
      publishMode: 'manual',
    })
    await post('/exam/publish', { examId: exam.data.id, expectedVersion: exam.data.version })
    message.value = '考试已发布。成绩需要教师发布后，学生才能看到分数。'
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

async function start(exam) {
  error.value = ''
  try {
    const res = await post('/exam/start', { examId: exam.id }, { 'Idempotency-Key': `${exam.id}-${Date.now()}` })
    attempt.value = res.data
    choices.value = {}
  } catch (err) {
    error.value = errorText(err)
  }
}

function optionList(question) {
  const raw = question.options
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'string') {
    try { return JSON.parse(raw) } catch { return [] }
  }
  return []
}

async function submitAttempt() {
  error.value = ''
  try {
    for (const question of attempt.value.questions || []) {
      await post('/exam/saveAnswer', {
        attemptId: attempt.value.id,
        questionId: question.id,
        expectedVersion: 0,
        answer: { choice: choices.value[question.id] || '' },
      })
    }
    const res = await post('/exam/submit', { attemptId: attempt.value.id })
    attempt.value = res.data
    message.value = res.data.score == null ? '已交卷。分数尚未发布。' : `得分 ${res.data.score}`
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
      <h1 class="page-title">考试</h1>
      <p class="lede">开始考试需要幂等键。未发布的成绩不会显示给考生。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <el-alert v-if="message" :title="message" type="success" show-icon :closable="false" />
    <div class="card stack" v-if="session.canTeach">
      <h3>发布客观题考试</h3>
      <div class="row">
        <el-input class="grow" v-model="form.title" placeholder="考试标题" />
        <el-select class="grow" v-model="form.questionId" placeholder="选择题目">
          <el-option v-for="q in questions" :key="q.id" :label="q.stem" :value="q.id" />
        </el-select>
        <el-button type="primary" @click="publishExam">发布</el-button>
      </div>
    </div>
    <div class="grid">
      <div class="card stack" v-for="exam in exams" :key="exam.id">
        <h3>{{ exam.title }}</h3>
        <span class="pill">{{ exam.status }}</span>
        <el-button v-if="exam.status === 'published'" @click="start(exam)">开始</el-button>
      </div>
    </div>
    <div class="card stack" v-if="attempt">
      <h3>答卷 {{ attempt.status }}</h3>
      <div v-for="question in attempt.questions || []" :key="question.id" class="stack">
        <strong>{{ question.stem }}</strong>
        <el-radio-group v-model="choices[question.id]">
          <el-radio v-for="opt in optionList(question)" :key="opt.key" :value="opt.key">{{ opt.key }}. {{ opt.text }}</el-radio>
        </el-radio-group>
      </div>
      <el-button type="primary" @click="submitAttempt">交卷</el-button>
    </div>
  </div>
</template>
