# Задача: сделать интерфейс десктопного приложения «synchronize»

Ты делаешь UI для готового десктопного приложения. Бэкенд уже написан и работает — твоя задача **только фронтенд**.

---

## 1. Что это за приложение

`synchronize` — утилита, которая держит git-репозитории синхронными между несколькими компьютерами. Человек работает на домашнем ПК, уходит, садится за рабочий — и код уже там. Приложение само делает `git pull` при старте, `git push` при выходе и синкает по таймеру.

Пользователь — разработчик. Приложение стоит в трее и открывается, когда надо посмотреть статус или нажать «синхронизировать сейчас».

---

## 2. Жёсткие технические ограничения

Приложение построено на **Wails v2** (Go + системный WebView). Это значит:

- **Ровно три файла:** `index.html`, `style.css`, `app.js`
- **Только vanilla** — никакого React, Vue, Svelte, Tailwind, TypeScript
- **Никаких сборщиков** — файлы отдаются как есть
- **Никаких CDN и внешних ссылок** — ни шрифтов Google, ни иконок с unpkg. Всё инлайном: SVG-иконки прямо в разметке, шрифты только системные (`-apple-system, "Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif`)
- **Движок — Chromium (WebView2)**, поэтому современный CSS/JS можно смело: grid, flex, `:has()`, optional chaining, `??`
- Файл `logo.png` уже лежит рядом — вставляй как `<img src="logo.png">`
- Скрипт подключается в конце `<body>`: `<script src="app.js"></script>`

### Важная деталь запуска

Wails инжектит свой рантайм до выполнения твоего скрипта, но `DOMContentLoaded` к этому моменту уже может пройти. Поэтому инициализацию оформи так:

```js
if (document.readyState === "loading")
  document.addEventListener("DOMContentLoaded", init);
else
  init();
```

---

## 3. API бэкенда

Все методы доступны как `window.go.main.App.<Method>(...)` и возвращают Promise. Удобно завести хелпер: `const go = () => window.go.main.App;`

### Чтение состояния

```js
await go().GetState(fetch)          // fetch: bool — дёргать ли сеть (медленно)
await go().GetDetail(name)          // подробности одного репозитория
await go().RecentLog(n)             // → string[]  последние n строк лога
await go().WatchedNames()           // → string[]  какие репо сейчас под наблюдением
await go().ListGitHubRepos()        // → string[]  ["owner/repo", ...] для добавления
```

### Действия над репозиторием

```js
await go().Pull(name)               // → OpResult
await go().Push(name)               // → OpResult
await go().Sync(name)               // → OpResult   (pull, затем push)
await go().SyncAll()                // → OpResult[]
await go().Diff(name)               // → string     текст diff
await go().SetEnabled(name, bool)   // включить/выключить автосинк репо
await go().SetInterval(name, mins)  // интервал автосинка в минутах
await go().RemoveRepo(name)         // убрать из приложения (файлы не трогает)
```

### Добавление репозиториев

```js
await go().PickFolder()             // → string    системный диалог выбора папки
await go().AddLocal(path)           // добавить существующую папку с git
await go().CloneAndAdd(slug, dest)  // склонировать с GitHub; dest можно ""
```

### Аккаунт и наблюдение

```js
await go().SignIn(pat)              // → string  имя пользователя; кидает ошибку если токен плохой
await go().SignOut()
await go().ToggleWatch()            // → bool    включает/выключает фоновый watcher
await go().OpenFolder(path)         // открыть папку в проводнике
await go().OpenOnGitHub(slug)       // открыть репо в браузере
```

### События (push из бэкенда)

```js
const rt = () => window.runtime;
rt().EventsOn("log",     (line) => {});   // новая строка лога
rt().EventsOn("changed", (name) => {});   // репозиторий изменился — перечитай состояние
rt().EventsOn("error",   (msg)  => {});   // показать ошибку
rt().BrowserOpenURL(url);                 // открыть ссылку во внешнем браузере
```

---

## 4. Формы данных

```js
// GetState(fetch) →
{
  repos: [ /* RepoView, см. ниже */ ],
  account:   "Falcon",                       // имя в GitHub
  loggedIn:  true,
  watching:  false,                          // работает ли фоновый watcher
  version:   "v1.0.1",
  identity:  "Falcon <mail@example.com>",    // подпись для коммитов
  configDir: "C:\\Users\\sasha\\.synchronize"
}

// RepoView
{
  name:     "Telegram-Discord-Roblox",       // уникальный ключ во всех вызовах
  path:     "E:\\Telegram-Discord-Roblox",
  slug:     "FalconTechDev/Telegram-Discord-Roblox", // может быть "" для локальных
  branch:   "main",
  remote:   "origin",
  tracking: "origin/main",
  ahead:    0,        // коммитов готово к push
  behind:   0,        // коммитов готово к pull
  dirty:    false,    // есть незакоммиченные правки
  changed:  0,        // сколько файлов изменено
  enabled:  true,     // участвует ли в автосинке
  private:  true,
  interval: 10,       // минут между автосинками
  lastSync: "8m ago", // уже человекочитаемо, или "" если никогда
  conflict: "",       // непустое = конфликт, текст = причина
  busy:     false,    // операция выполняется прямо сейчас
  error:    ""        // непустое = что-то сломано
}

// GetDetail(name) → всё из RepoView плюс:
{
  commits: [ { hash: "a89d86d", subject: "fix login", author: "Falcon", when: "2 hours ago" } ],
  incoming: ["down  1a2b3c4  add readme"],   // готовые строки, просто выводи
  outgoing: ["up    a89d86d  sync: auto-commit"],
  files:    [ { status: "modified", path: "src/main.go" } ]
  // status: "new" | "modified" | "deleted" | "added" | "renamed"
}

// OpResult
{
  ok: true,
  summary: "pulled 1 commit from origin/main",  // короткая фраза для тоста
  lines:   ["down  1a2b3c4  add readme"],       // детали для лога
  error:   ""                                   // непустое = провал
}
```

---

## 5. Что нужно построить

### Экран входа

Показывается когда `loggedIn === false`. Логотип, название, поле для GitHub Personal Access Token (`type="password"`), кнопка подключения, ссылка «создать токен» (ведёт на `https://github.com/settings/tokens/new?scopes=repo&description=synchronize` через `rt().BrowserOpenURL`). Внизу мелко: токен хранится только на этом компьютере, нужен scope `repo`.

### Основной экран

**Титульная строка** — логотип, название, версия. Обязательно поставь `--wails-draggable: drag` на неё (за неё таскают окно) и `--wails-draggable: no-drag` на кнопки внутри.

**Список репозиториев** слева. Для каждого: имя, когда синкался, цветовой индикатор состояния и компактный счётчик (`↓3`, `↑1`). Выбор репозитория открывает его справа.

**Рабочая область** — по выбранному репозиторию:

- Имя, ветка/tracking, путь (клик открывает папку)
- Кнопки Pull / Push / Sync и доступ к настройкам
- Числа: сколько к pull, сколько к push, сколько незакоммичено, когда синкался
- Что ждёт отправки/получения (`incoming`/`outgoing`)
- Незакоммиченные файлы со статусами
- Последние коммиты

**Наблюдение (watch).** Сейчас это одна глобальная кнопка — сделай понятнее: переключатель у каждого репозитория (`SetEnabled`), а глобальная кнопка запускает/останавливает watcher целиком (`ToggleWatch`). Должно быть очевидно, какие репозитории под наблюдением, а какие на паузе.

**Лог активности** — сворачиваемая панель внизу. Строки приходят событием `log`. Раскрашивай: `[ERROR]`/`FAILED`/`CONFLICT` — красным, `[WARN]` — жёлтым, `pushed`/`pulled`/`committed` — зелёным.

**Диалоги:** добавление репозитория (две вкладки — клонировать с GitHub через `ListGitHubRepos` или указать существующую папку через `PickFolder`), настройки репозитория (интервал, вкл/выкл, удаление), просмотр diff (моноширинный, `+` зелёным, `-` красным).

**Тосты** для результатов операций.

---

## 6. Как это должно выглядеть

**Чёрная палитра.** Фон именно чёрный (`#000`), не серо-синий. Поверхности чуть светлее (`#0a0a0a`, `#131313`), границы едва различимы (`#1e1e1e`). Один акцентный цвет — фиолетовый `#8b7bff`. Состояния: зелёный `#3ddc84` (в порядке), жёлтый `#f0b429` (внимание), красный `#ff5f5f` (ошибка), голубой `#4aa8ff` (входящее).

Ориентир по вкусу — Linear, Vercel dashboard, GitHub Desktop: спокойно, плотно, много воздуха между блоками, ничего кричащего. Типографика мелкая (12–14px основной текст), заголовки секций — крошечные капсом с разрядкой. Скругления 8–12px. Переходы быстрые (100–150ms), без пружин и отскоков.

**Важно:** окно бывает узким (от 820px). Ничего не должно вылезать за край и появляться горизонтальный скролл. Длинные пути и имена обрезай многоточием. У flex/grid-детей ставь `min-width: 0`, иначе они не сжимаются.

Мелочи, которые делают ощущение готового продукта: индикатор занятости во время операции, аккуратные пустые состояния («ещё нет репозиториев» с кнопкой добавить), disabled на кнопках во время работы, `user-select: none` на интерфейсе но `text` на логах/путях/diff, кастомный тонкий скроллбар.

---

## 7. Что отдать

Три файла целиком: `index.html`, `style.css`, `app.js`. Рабочий код, а не наброски — я вставлю его в проект как есть.
