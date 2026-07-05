<template>
    <div class="app">
        <!-- Страница логина/регистрации -->
        <div v-if="!isLoggedIn" class="auth-page">
            <!-- Переключатель между входом и регистрацией -->
            <div class="auth-tabs">
                <button
                    @click="isLogin = true"
                    :class="{ active: isLogin }"
                >
                    Вход
                </button>
                <button
                    @click="isLogin = false"
                    :class="{ active: !isLogin }"
                >
                    Регистрация
                </button>
            </div>

            <!-- Форма входа -->
            <div v-if="isLogin" class="login-form">
                <h1>Вход в чат</h1>
                <input v-model="email" placeholder="Email" type="text" />
                <input v-model="password" placeholder="Пароль" type="password" />
                <button @click="login">Войти</button>
            </div>

            <!-- Форма регистрации -->
            <div v-else class="register-form">
                <h1>Регистрация</h1>
                <input v-model="registerName" placeholder="Имя" type="text" />
                <input v-model="registerSurname" placeholder="Фамилия" type="text" />
                <input v-model="registerEmail" placeholder="Email" type="email" />
                <input v-model="registerPassword" placeholder="Пароль" type="password" />
                <button @click="register">Зарегистрироваться</button>
            </div>
        </div>

        <!-- Страница с чатами -->
        <div v-else class="chats-page">
            <h2>Ваши чаты</h2>
            <div class="create-chat-buttons">
                <button @click="showCreateDirectChat = true">Создать личный чат</button>
                <button @click="showCreateGroupChat = true">Создать групповой чат</button>
            </div>

            <!-- Создание личного чата -->
            <div v-if="showCreateDirectChat" class="create-chat-form">
                <h3>Создать личный чат</h3>
                <input
                    v-model="newDirectChatEmail"
                    placeholder="Email собеседника"
                    type="email"
                >
                <div class="form-buttons">
                    <button @click="createDirectChat">Создать</button>
                    <button @click="showCreateDirectChat = false">Отмена</button>
                </div>
            </div>

            <!-- Создание группового чата -->
            <div v-if="showCreateGroupChat" class="create-chat-form">
                <h3>Создать групповой чат</h3>
                <input
                    v-model="newGroupChatName"
                    placeholder="Название группы"
                    type="text"
                >

                <!-- Поиск участников -->
                <input
                    v-model="participantSearch"
                    @input="searchUsers"
                    placeholder="Поиск пользователей по email или имени"
                    type="text"
                >

                <!-- Результаты поиска -->
                <div v-if="searchResults.length > 0" class="search-results">
                    <div
                        v-for="user in searchResults"
                        :key="user.uuid"
                        @click="addParticipant(user)"
                        class="user-item"
                    >
                        {{ user.name }} ({{ user.email }})
                    </div>
                </div>

                <!-- Список выбранных участников -->
                <div v-if="selectedParticipants.length > 0" class="selected-participants">
                    <h4>Участники:</h4>
                    <div
                        v-for="(participant, index) in selectedParticipants"
                        :key="participant.uuid"
                        class="participant-tag"
                    >
                        {{ participant.name }}
                        <button @click="removeParticipant(index)">×</button>
                    </div>
                </div>

                <div class="form-buttons">
                    <button @click="createGroupChat">Создать</button>
                    <button @click="showCreateGroupChat = false">Отмена</button>
                </div>
            </div>
            <div v-if="chats.length === 0" class="empty">
                У вас пока нет чатов
            </div>
            <div v-else class="chats-list">
                <button
                    v-for="chat in chats"
                    :key="chat.chat_uuid"
                    @click="openChat(chat.chat_uuid)"
                    class="chat-item chat-button"
                >
                    {{ getChatDisplayName(chat) }}
                </button>
            </div>

            <button @click="logout">Выйти</button>
        </div>


        <!-- Интерфейс чата -->
        <div v-if="currentChatUuid" class="chat-interface">
            <div class="chat-header">
                <h3>{{ getCurrentChatName() }}</h3>
                <button @click="closeChat">Закрыть</button>
            </div>

            <!-- Сообщения -->
            <div class="messages" ref="messagesContainer">
                <div
                    v-for="message in messages"
                    :key="message.uuid || message.chat_uuid + '-' + message.created_at"
                    class="message"
                >
                    <strong>{{ getSenderName(message) }}:</strong> {{ message.content || message.text }}
                </div>
            </div>

            <!-- Форма отправки -->
            <div class="message-input">
                <input
                    v-model="newMessage"
                    @keyup.enter="sendMessage"
                    placeholder="Введите сообщение..."
                >
                <button @click="sendMessage" :disabled="!newMessage.trim()">
                    Отправить
                </button>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import axios from 'axios'

const api = axios.create({ baseURL: '/' })

api.interceptors.request.use(config => {
    const token = localStorage.getItem('token')
    if (token) config.headers.Authorization = `Bearer ${token}`
    return config
})

const email = ref('test@test.ru')
const password = ref('123456')
const chats = ref([])
const isLoggedIn = ref(!!localStorage.getItem('token'))
const isLogin = ref(true)

const registerName = ref('')
const registerSurname = ref('')
const registerEmail = ref('')
const registerPassword = ref('')

const messages = ref([])
const newMessage = ref('')
const currentUserUUID = ref(null)
const currentChatUuid = ref(null)

const showCreateDirectChat = ref(false)
const showCreateGroupChat = ref(false)
const newDirectChatEmail = ref('')
const newGroupChatName = ref('')

const participantSearch = ref('')
const searchResults = ref([])
const selectedParticipants = ref([])

const login = async () => {
    try {
        const res = await api.post('/api/v1/login', { email: email.value, password: password.value })
        localStorage.setItem('token', res.data.access_token)
        isLoggedIn.value = true

        const tokenParts = res.data.access_token.split('.')
        const payload = JSON.parse(atob(tokenParts[1]))
        currentUserUUID.value = payload.user_uuid

        await loadChats()
    } catch (err) {
        alert('Ошибка входа: ' + (err.response?.data?.error || 'Неизвестная ошибка'))
    }
}

const register = async () => {
    if (!registerName.value.trim() || !registerSurname.value.trim() ||
        !registerEmail.value.trim() || !registerPassword.value.trim()) {
        alert('Заполните все поля')
        return
    }

    if (registerPassword.value.length < 6) {
        alert('Пароль должен быть не менее 6 символов')
        return
    }

    try {
        await api.post('/api/v1/register', {
            name: registerName.value,
            surname: registerSurname.value,
            email: registerEmail.value,
            password: registerPassword.value
        })

        alert('Регистрация успешна! Теперь войдите в систему.')
        isLogin.value = true
        email.value = registerEmail.value
        password.value = ''

        registerName.value = ''
        registerSurname.value = ''
        registerEmail.value = ''
        registerPassword.value = ''
    } catch (err) {
        alert('Ошибка регистрации: ' + (err.response?.data?.error || 'Неизвестная ошибка'))
    }
}

const loadChats = async () => {
    try {
        const res = await api.get('/api/v1/chats')
        console.log('Загружены чаты:', res.data)
        chats.value = res.data.chats || []
        console.log('chats.value:', chats.value)
    } catch (err) {
        console.error('Ошибка загрузки чатов:', err)
    }
}


const createDirectChat = async () => {
    if (!newDirectChatEmail.value.trim()) {
        alert('Введите email собеседника')
        return
    }

    try {
        await api.post('/api/v1/chats/direct', {
            with_email: newDirectChatEmail.value
        })

        alert('Чат создан!')
        showCreateDirectChat.value = false
        newDirectChatEmail.value = ''
        await loadChats()
    } catch (err) {
        alert('Ошибка: ' + (err.response?.data?.error || 'Неизвестная ошибка'))
    }
}

const searchUsers = async () => {
    if (participantSearch.value.length < 2) {
        searchResults.value = []
        return
    }

    try {
        const res = await api.get(`/api/v1/users/search?q=${participantSearch.value}`)
        searchResults.value = res.data.users || []
    } catch (err) {
        console.error('Ошибка поиска:', err)
    }
}

const addParticipant = (user) => {
    if (!selectedParticipants.value.find(p => p.uuid === user.uuid)) {
        selectedParticipants.value.push(user)
    }
    participantSearch.value = ''
    searchResults.value = []
}

const removeParticipant = (index) => {
    selectedParticipants.value.splice(index, 1)
}

const createGroupChat = async () => {
    if (!newGroupChatName.value.trim()) {
        alert('Введите название группы')
        return
    }

    try {
        const participantUUIDs = selectedParticipants.value.map(p => p.uuid)

        await api.post('/api/v1/chats/group', {
            name: newGroupChatName.value,
            participants: participantUUIDs
        })

        alert('Групповой чат создан!')
        showCreateGroupChat.value = false
        newGroupChatName.value = ''
        selectedParticipants.value = []
        participantSearch.value = ''
        searchResults.value = []
        await loadChats()
    } catch (err) {
        console.error('Ошибка создания группового чата:', err)
        alert('Ошибка: ' + (err.response?.data?.error || 'Неизвестная ошибка'))
    }
}

const getChatDisplayName = (chat) => {
    if (chat.name) {
        return chat.name
    }
    if (chat.type === 'direct') {
        return chat.participant_name || 'Личный чат'
    }
    return 'Групповой чат'
}

const openChat = async (chatUuid) => {
    currentChatUuid.value = chatUuid
    messages.value = []

    try {
        const res = await api.get(`/api/v1/chats/${chatUuid}/messages`)
        messages.value = res.data.messages || []
        scrollToBottom()
    } catch (err) {
        console.error('Ошибка загрузки истории:', err)
    }

    try {
        await api.get(`/api/v1/chats/${chatUuid}/read`)
    } catch (err) {
        console.error('Ошибка пометки как прочитанное:', err)
    }

    connectWebSocket(chatUuid)
}

const closeChat = () => {
    currentChatUuid.value = null
    messages.value = []
    newMessage.value = ''
    disconnectWebSocket()
}

const sendMessage = async () => {
    if (!newMessage.value.trim()) return

    const messageText = newMessage.value.trim()
    newMessage.value = ''

    const tempMessage = {
        uuid: 'temp-' + Date.now(),
        chat_uuid: currentChatUuid.value,
        sender_uuid: currentUserUUID.value,
        sender_name: 'Вы',
        content: messageText,
        created_at: new Date().toISOString(),
        is_read: false
    }
    messages.value.push(tempMessage)
    scrollToBottom()

    try {
        await api.post('/api/v1/message', {
            chat_uuid: currentChatUuid.value,
            text: messageText
        })
    } catch (err) {
        console.error('Ошибка отправки сообщения:', err)
        alert('Ошибка отправки сообщения')
        messages.value = messages.value.filter(m => m.uuid !== tempMessage.uuid)
    }
}

const getCurrentChatName = () => {
    const chat = chats.value.find(c => c.chat_uuid === currentChatUuid.value)
    return chat ? getChatDisplayName(chat) : 'Чат'
}

const getSenderName = (message) => {
    if (message.sender_uuid === currentUserUUID.value) {
        return 'Вы'
    }
    return message.sender_name || 'Пользователь'
}

const scrollToBottom = () => {
    nextTick(() => {
        const container = document.querySelector('.messages')
        if (container) {
            container.scrollTop = container.scrollHeight
        }
    })
}

const logout = () => {
    localStorage.removeItem('token')
    isLoggedIn.value = false
    chats.value = []
    currentUserUUID.value = null
    closeChat()
}

// WebSocket для реального времени
let wsConnection = null

const connectWebSocket = (chatUuid) => {
    if (wsConnection) {
        wsConnection.close()
    }

    const backendUrl = window.location.hostname === 'localhost'
        ? 'ws://127.0.0.1:8080'
        : 'ws://192.168.0.10:8080'
    const token = localStorage.getItem('token')
    const wsUrl = `ws://${backendUrl}/ws/chat/${chatUuid}?token=${token}`

    wsConnection = new WebSocket(wsUrl)

    wsConnection.onopen = () => {
        console.log('WebSocket подключен')
    }

    wsConnection.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data)

            if (data.type === 'history') {
                return
            }

            if (data.chat_uuid === currentChatUuid.value) {
                messages.value = messages.value.filter(m => !m.uuid.startsWith('temp-'))

                const exists = messages.value.some(m => m.uuid === data.uuid)
                if (!exists) {
                    messages.value.push({
                        uuid: data.uuid,
                        chat_uuid: data.chat_uuid,
                        sender_uuid: data.sender_uuid,
                        sender_name: data.sender_name,
                        content: data.content,
                        created_at: data.created_at,
                        is_read: data.is_read
                    })
                    scrollToBottom()
                }
            }
        } catch (err) {
            console.error('Ошибка обработки сообщения WebSocket:', err)
        }
    }

    wsConnection.onclose = () => {
        console.log('WebSocket отключен')
    }

    wsConnection.onerror = (error) => {
        console.error('WebSocket ошибка:', error)
    }
}

const disconnectWebSocket = () => {
    if (wsConnection) {
        wsConnection.close()
        wsConnection = null
    }
}

onMounted(() => {
    if (isLoggedIn.value) {
        const token = localStorage.getItem('token')
        if (token) {
            try {
                const tokenParts = token.split('.')
                const payload = JSON.parse(atob(tokenParts[1]))
                currentUserUUID.value = payload.user_uuid
            } catch (e) {
                console.error('Ошибка парсинга токена:', e)
            }
        }
        loadChats()
    }
})
</script>
