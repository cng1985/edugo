<script setup>
import { onMounted, reactive, ref } from 'vue'
import { post, errorText } from '../api'
import { useSession } from '../session'

const session = useSession()
const courses = ref([])
const current = ref(null)
const error = ref('')
const form = reactive({ title: '', summary: '' })
const chapterTitle = ref('新章节')
const lesson = reactive({ title: '新课时', lessonType: 'document', contentText: '请阅读本页内容。' })

async function load() {
  const res = await post('/course/list', { page: 1, size: 50 })
  courses.value = res.data || []
}

async function openCourse(row) {
  const res = await post('/course/detail', { courseId: row.id })
  current.value = res.data
}

async function createCourse() {
  error.value = ''
  try {
    const res = await post('/course/create', { title: form.title, summary: form.summary })
    form.title = ''
    form.summary = ''
    await load()
    current.value = res.data
  } catch (err) {
    error.value = errorText(err)
  }
}

async function addChapter() {
  const res = await post('/chapter/create', {
    courseId: current.value.id,
    expectedVersion: current.value.version,
    title: chapterTitle.value,
    sortOrder: (current.value.chapters || []).length + 1,
  })
  current.value = res.data
}

async function addLesson(chapter) {
  const res = await post('/lesson/create', {
    courseId: current.value.id,
    chapterId: chapter.id,
    expectedVersion: current.value.version,
    title: lesson.title,
    lessonType: lesson.lessonType,
    contentText: lesson.contentText,
    sortOrder: (chapter.lessons || []).length + 1,
    required: true,
  })
  current.value = res.data
}

async function publish() {
  error.value = ''
  try {
    const res = await post('/course/publish', { courseId: current.value.id, expectedVersion: current.value.version })
    current.value = res.data
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
      <h1 class="page-title">课程</h1>
      <p class="lede">已发布课程对学习者可见。发布后的修订会生成新草稿，已发布版本保持不变。</p>
    </div>
    <el-alert v-if="error" :title="error" type="error" show-icon :closable="false" />
    <div class="card stack" v-if="session.canTeach">
      <h3>新建课程</h3>
      <div class="row">
        <el-input class="grow" v-model="form.title" placeholder="标题" />
        <el-input class="grow" v-model="form.summary" placeholder="简介" />
        <el-button type="primary" @click="createCourse">创建</el-button>
      </div>
    </div>
    <div class="grid">
      <button class="card" v-for="course in courses" :key="course.id" @click="openCourse(course)">
        <h3>{{ course.title }}</h3>
        <span class="pill">{{ course.status }}</span>
        <p class="muted">{{ course.summary }}</p>
      </button>
    </div>
    <div class="card stack" v-if="current">
      <h3>{{ current.title }}</h3>
      <div class="row">
        <span class="pill">{{ current.status }}</span>
        <span class="muted">版本 {{ current.version }}</span>
      </div>
      <div v-for="chapter in current.chapters || []" :key="chapter.id" class="card">
        <strong>{{ chapter.title }}</strong>
        <p v-for="item in chapter.lessons || []" :key="item.id">{{ item.title }} · {{ item.lessonType }}</p>
        <div class="row" v-if="session.canTeach && current.status !== 'archived'">
          <el-input class="grow" v-model="lesson.title" placeholder="课时标题" />
          <el-select v-model="lesson.lessonType" style="width: 120px">
            <el-option label="文档" value="document" />
            <el-option label="练习" value="exercise" />
          </el-select>
          <el-button @click="addLesson(chapter)">添加课时</el-button>
        </div>
      </div>
      <div class="row" v-if="session.canTeach">
        <el-input class="grow" v-model="chapterTitle" placeholder="章节标题" />
        <el-button @click="addChapter">添加章节</el-button>
        <el-button type="primary" @click="publish">发布</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
button.card { text-align: left; cursor: pointer; color: inherit; }
</style>
