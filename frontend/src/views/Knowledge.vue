<script setup>
import { reactive, ref } from 'vue'
import { post, errorText } from '../api'
import { useSession } from '../session'

const session = useSession()
const query = ref('')
const items = ref([])
const mode = ref('')
const error = ref('')
const form = reactive({ name: '', description: '', text: '' })

async function search() {
  error.value = ''
  try {
    const res = await post('/aiKnowledge/search', { query: query.value, topK: 5 })
    items.value = res.data.items || []
    mode.value = res.data.mode
  } catch (err) {
    error.value = errorText(err)
  }
}

async function create() {
  error.value = ''
  try {
    await post('/knowledge/create', { name: form.name, description: form.description })
    if (form.text) {
      await post('/aiKnowledge/index', { courseId: form.name, text: form.text })
    }
    form.name = ''
    form.description = ''
    form.text = ''
  } catch (err) {
    error.value = errorText(err)
  }
}
</script>

<template>
  <div class="stack">
    <div>
      <h1 class="page-title">知识</h1>
      <p class="lede">没有向量模型时按关键词检索，并标明检索方式。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card row">
      <el-input class="grow" v-model="query" placeholder="搜索已索引内容" @keyup.enter="search" />
      <el-button type="primary" @click="search">搜索</el-button>
    </div>
    <p class="muted" v-if="mode">检索方式：{{ mode }}</p>
    <div class="card" v-for="item in items" :key="item.id">{{ item.text }}</div>
    <div class="card stack" v-if="session.canTeach">
      <h3>添加知识点</h3>
      <el-input v-model="form.name" placeholder="名称" />
      <el-input v-model="form.description" placeholder="说明" />
      <el-input v-model="form.text" type="textarea" placeholder="可检索正文" />
      <el-button @click="create">保存</el-button>
    </div>
  </div>
</template>
