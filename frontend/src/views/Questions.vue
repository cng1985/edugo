<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'

const rows = ref([])
const error = ref('')
const form = reactive({
  type: 'single',
  stem: '',
  difficulty: 'easy',
  choice: 'A',
  optionA: '正确',
  optionB: '错误',
})

async function load() {
  const res = await post('/question/list', { page: 1, size: 50 })
  rows.value = res.data || []
}

async function create() {
  error.value = ''
  try {
    const created = await post('/question/create', {
      type: form.type,
      stem: form.stem,
      difficulty: form.difficulty,
      maxScoreCenti: 1000,
      options: [
        { key: 'A', text: form.optionA },
        { key: 'B', text: form.optionB },
      ],
      answer: { choice: form.choice },
    })
    const reviewed = await post('/question/submitReview', { questionId: created.data.id, expectedVersion: created.data.version })
    await post('/question/approve', { questionId: created.data.id, expectedVersion: reviewed.data.version })
    form.stem = ''
    await load()
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
      <h1 class="page-title">题库</h1>
      <p class="lede">单选题会先提交审核，再标记为通过，之后才能进入试卷。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card stack">
      <el-input v-model="form.stem" placeholder="题干" />
      <div class="row">
        <el-input class="grow" v-model="form.optionA" placeholder="选项 A" />
        <el-input class="grow" v-model="form.optionB" placeholder="选项 B" />
        <el-select v-model="form.choice" style="width: 120px">
          <el-option label="答案 A" value="A" />
          <el-option label="答案 B" value="B" />
        </el-select>
        <el-button type="primary" @click="create">保存并通过</el-button>
      </div>
    </div>
    <div class="card" v-for="row in rows" :key="row.id">
      <strong>{{ row.stem }}</strong>
      <div class="muted">{{ row.type }} · {{ row.reviewStatus }} · 版本 {{ row.versionNo }}</div>
    </div>
  </div>
</template>
