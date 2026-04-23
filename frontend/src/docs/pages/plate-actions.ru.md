# Интерактивные плашки (plate actions)

Авторы карточек могут вставлять в сообщения кликабельные кнопки через `data-nest-action` — без JavaScript'а и без XSS-рисков. WuNest сканирует DOM сообщения и навешивает обработчики только на разрешённые действия из whitelist'а.

## Как это работает

Добавь атрибут `data-nest-action="имя-действия"` к `<button>` (или любому элементу) внутри HTML-плашки. Дополнительные параметры — через `data-nest-*` атрибуты. Никакого `onclick` или `<script>` — всё что там — DOMPurify всё равно вырежет.

```html
<!-- Простой пример — копирование текста -->
<button data-nest-action="copy" data-nest-text="Hello, world!">📋 Copy</button>

<!-- Бросок кубика -->
<button data-nest-action="dice" data-nest-dice="2d6">🎲 Roll 2d6</button>
```

## Доступные действия

### Системные (toast-подтверждение)

| Действие | Параметры | Что делает |
|---|---|---|
| `copy` | `data-nest-text` (литерал) или `data-nest-target` (CSS селектор) | Копирует в clipboard; без параметров — копирует innerText кнопки |
| `dice` | `data-nest-dice="2d6"` или `data-nest-sides="20"` | Бросает кубики; результат в toast или в `data-nest-result-to` селектор |
| `reroll` | Как `dice` | Алиас |

### Контекст сообщения (bubble up)

| Действие | Что делает |
|---|---|
| `swipe-prev` | Предыдущий swipe этого сообщения (для alternate_greetings в первом) |
| `swipe-next` | Следующий swipe |
| `regenerate` | Перегенерация сообщения (только для последнего assistant'а) |
| `edit` | Открыть режим редактирования |
| `delete` | Удалить сообщение |

### Композер (вставка текста)

| Действие | Параметры | Что делает |
|---|---|---|
| `say` | `data-nest-text` или innerText | Заполняет поле ввода (юзер может отредактировать и послать) |
| `send` | `data-nest-text` или innerText | Заполняет и сразу отправляет сообщение |

### Локальные DOM-переключатели

| Действие | Параметры | Что делает |
|---|---|---|
| `toggle-attr` | `data-nest-target` (селектор), `data-nest-attr="hidden"` | Переключает атрибут на целевом элементе |
| `toggle-class` | `data-nest-target`, `data-nest-class="expanded"` | Переключает класс |

## Полные примеры

### Меню действий персонажа

```html
<div class="char-actions">
  <button data-nest-action="say" data-nest-text="Я отхожу назад и прислушиваюсь.">
    👂 Прислушаться
  </button>
  <button data-nest-action="say" data-nest-text="Я делаю шаг вперёд с мечом наготове.">
    ⚔️ Атаковать
  </button>
  <button data-nest-action="say" data-nest-text="Я пытаюсь убежать.">
    🏃 Бежать
  </button>
</div>
```

### Stat block с броском на проверку

```html
<div class="stat-panel">
  <h3>Проверка Ловкости</h3>
  <p>Модификатор: +3, DC 15</p>
  <button data-nest-action="dice" data-nest-dice="1d20" data-nest-result-to="#roll-result">
    🎲 Бросить d20
  </button>
  <div id="roll-result" class="roll-result">(жми кнопку)</div>
</div>
```

### Копирование stat-блока

```html
<div id="stat-block">
  <p>HP: 50/50 · MP: 30/30 · STR 14 · DEX 16</p>
</div>
<button data-nest-action="copy" data-nest-target="#stat-block">📋 Копировать статы</button>
```

### Раскрывашка доп. инфо (без `<details>`)

```html
<button data-nest-action="toggle-attr" data-nest-target="#lore-extra" data-nest-attr="hidden">
  📖 Показать/скрыть подробности
</button>
<div id="lore-extra" hidden>
  <p>Дополнительный lore…</p>
</div>
```

### Кнопка «Следующее приветствие»

```html
<button data-nest-action="swipe-next">→ Другое приветствие</button>
```

## Ограничения и безопасность

- `<script>`, `onclick=`, `javascript:` и подобные вырезаются DOMPurify'ем всегда — это не обходится
- Whitelist действий фиксированный; неизвестные имена тихо игнорируются
- `data-nest-target` — обычный CSS-селектор, работает только в пределах того же сообщения
- Bubble-up действия (`swipe-next` и т.п.) доступны только на assistant-сообщениях с соответствующей возможностью
- Кубики бросаются на клиенте (через `crypto.getRandomValues`) — на сервер ничего не уходит

## Для более сложного интерактива

Если нужен полноценный JS (state machines, API-вызовы) — это не поддерживается из соображений безопасности и не планируется. Используй doкументированный whitelist, а для state — CSS-only техники (`:checked ~ sibling`, `:target`, `<details>`) они работают без доработок.
