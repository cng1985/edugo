<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'

const plans = ref([])
const paths = ref([])
const error = ref('')
const form = reactive({ title: '', deadline: '', item: '' })
const goal = ref('')

async function load() {
  const [planRes, pathRes] = await Promise.all([post('/studyPlan/list', {}), post('/learningPath/list', {})])
  plans.value = planRes.data || []
  paths.value = pathRes.data || []
}

async function create() {
  error.value = ''
  try {
    const deadlineAt = new Date(form.deadline).toISOString()
    await post('/studyPlan/create', {
      title: form.title,
      deadlineAt,
      items: [{ title: form.item, required: true, manual: true }],
    })
    form.title = ''
    form.item = ''
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

async function start(plan) {
  await post('/studyPlan/start', { planId: plan.id })
  await load()
}

async function generate() {
  error.value = ''
  try {
    await post('/learningPath/generate', { goal: goal.value })
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
      <h1 class="page-title">计划</h1>
      <p class="lede">学习计划由你确认后才会开始。AI 路径在模型未启用时不会生成草稿。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card stack">
      <div class="row">
        <el-input class="grow" v-model="form.title" placeholder="计划标题" />
        <el-input class="grow" v-model="form.item" placeholder="第一项" />
        <el-date-picker v-model="form.deadline" type="datetime" placeholder="截止时间" />
        <el-button type="primary" @click="create">创建</el-button>
      </div>
    </div>
    <div class="card row" v-for="plan in plans" :key="plan.id">
      <strong class="grow">{{ plan.title }}</strong>
      <span class="pill">{{ plan.status }}</span>
      <el-button v-if="plan.status === 'draft'" @click="start(plan)">开始</el-button>
    </div>
    <div class="card row">
      <el-input class="grow" v-model="goal" placeholder="想达成的学习目标" />
      <el-button @click="generate">生成路径草稿</el-button>
    </div>
    <div class="card" v-for="path in paths" :key="path.id">{{ path.title }} · {{ path.status }}</div>
  </div>
</template>
