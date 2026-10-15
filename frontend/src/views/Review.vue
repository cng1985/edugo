<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'

const due = ref([])
const stats = ref(null)
const form = reactive({ front: '', back: '' })
const error = ref('')

async function load() {
  const [cards, summary] = await Promise.all([post('/reviewCard/due', {}), post('/reviewCard/stats', {})])
  due.value = cards.data || []
  stats.value = summary.data
}

async function create() {
  error.value = ''
  try {
    await post('/reviewCard/create', form)
    form.front = ''
    form.back = ''
    await load()
  } catch (err) {
    error.value = errorText(err)
  }
}

async function grade(card, quality) {
  error.value = ''
  try {
    await post('/reviewCard/submit', {
      cardId: card.id,
      submissionId: `${card.id}-${Date.now()}`,
      quality,
      expectedVersion: card.version,
    })
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
      <h1 class="page-title">复习</h1>
      <p class="lede">评分使用 sm2-edugo-v1。4 分会把新卡片安排到明天。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card" v-if="stats">待复习 {{ stats.due }} · 学习中 {{ stats.learning }} · 复习中 {{ stats.review }} · 已掌握 {{ stats.mastered }}</div>
    <div class="card row">
      <el-input class="grow" v-model="form.front" placeholder="正面" />
      <el-input class="grow" v-model="form.back" placeholder="背面" />
      <el-button type="primary" @click="create">添加卡片</el-button>
    </div>
    <div class="card stack" v-for="card in due" :key="card.id">
      <strong>{{ card.front }}</strong>
      <p>{{ card.back }}</p>
      <div class="row">
        <el-button v-for="score in [0, 3, 4, 5]" :key="score" @click="grade(card, score)">{{ score }} 分</el-button>
      </div>
    </div>
  </div>
</template>
