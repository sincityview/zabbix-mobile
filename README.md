####  Zabbix Android API Client 
------

**Версия App**
```text
v0.8.3
```

**Версия Go**
```text
1.24.6
```

**Сборка Fyne**
```text
go install github.com/fyne-io/fyne-cross@latest

fyne-cross android -app-version 0.8.3 -app-id com.zabbix.android.monitor -icon icon.png \
  -tags no_emoji -arch arm64
```

**Приложение для Android**
- Нужно сгенерировать на стороне сервера Token (или использовать логин/пароль)
- Можно получать текущие проблемы
- Есть проверка подключения прямо в окне настроек
- Кеш последнего ответа: при запуске данные видны сразу, без ожидания сети
- Карточка триггера — всегда три строки фиксированной высоты (время, хост,
  проблема): длинные значения обрезаются посередине с сохранением значимого
  хвоста, а полный текст открывается тапом по карточке

<br>

<table>
  <tr>
    <td><img src="img/main.jpg" width="324" /></td>
    <td><img src="img/settings.jpg" width="324" /></td>
  </tr>
</table>
