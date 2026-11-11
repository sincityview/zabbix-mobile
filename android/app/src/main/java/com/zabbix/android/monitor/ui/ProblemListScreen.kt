package com.zabbix.android.monitor.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.zabbix.android.monitor.R
import com.zabbix.android.monitor.data.MonitorSettings
import com.zabbix.android.monitor.data.Problem
import com.zabbix.android.monitor.data.ProblemsPage
import com.zabbix.android.monitor.data.ZabbixApi
import com.zabbix.android.monitor.data.ZabbixTime
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Состояние экрана. Держим в обычном классе на `mutableStateOf`, а не в
 * ViewModel: экрану нужен один список и один признак ошибки, а переживать
 * поворот конфигурации будет повторный запрос.
 */
private class ProblemListState(private val settings: MonitorSettings) {

    private val api = ZabbixApi(settings.url, settings.token, settings.allowInsecureTls)

    var page by mutableStateOf<ProblemsPage?>(null)
        private set

    /** Список для отрисовки; пустой, пока не пришёл первый ответ. */
    val problems: List<Problem>
        get() = page?.shown.orEmpty()

    var error by mutableStateOf<String?>(null)
        private set
    var loading by mutableStateOf(false)
        private set

    /** Обновление, запущенное пользователем (кнопка или жест) — для индикатора. */
    var refreshing by mutableStateOf(false)
        private set
    var updatedAt by mutableStateOf(0L)
        private set

    suspend fun refresh(userInitiated: Boolean = false) {
        if (loading) return
        loading = true
        if (userInitiated) refreshing = true
        try {
            page = withContext(Dispatchers.IO) { api.problems(limit = settings.safeLimit()) }
            error = null
            updatedAt = System.currentTimeMillis()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            error = e.message ?: e.javaClass.simpleName
        } finally {
            loading = false
            refreshing = false
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProblemListScreen(settings: MonitorSettings, onOpenSettings: () -> Unit) {
    val state = remember(settings) { ProblemListState(settings) }
    val scope = rememberCoroutineScope()
    val lifecycleOwner = LocalLifecycleOwner.current
    var selected by remember { mutableStateOf<Problem?>(null) }

    val refresh: () -> Unit = { scope.launch { state.refresh(userInitiated = true) } }

    // Первый запрос при входе на экран.
    LaunchedEffect(state) { state.refresh() }

    // Автообновление с интервалом из настроек, только на переднем плане.
    val intervalMs = settings.safeIntervalSeconds() * 1000L
    LaunchedEffect(state, intervalMs) {
        while (true) {
            delay(intervalMs)
            if (lifecycleOwner.lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) {
                state.refresh()
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(problemsTitle(state)) },
                actions = {
                    IconButton(onClick = refresh, enabled = !state.loading) {
                        Icon(
                            imageVector = Icons.Filled.Refresh,
                            contentDescription = stringResource(R.string.update),
                        )
                    }
                    IconButton(onClick = onOpenSettings) {
                        Icon(
                            imageVector = Icons.Filled.Settings,
                            contentDescription = stringResource(R.string.settings),
                        )
                    }
                },
            )
        },
    ) { innerPadding ->
        PullToRefreshBox(
            isRefreshing = state.refreshing,
            onRefresh = refresh,
            modifier = Modifier.fillMaxSize().padding(innerPadding),
        ) {
            when {
                state.problems.isEmpty() && state.loading -> {
                    CircularProgressIndicator(Modifier.align(Alignment.Center))
                }

                state.problems.isEmpty() && state.error != null -> {
                    MessagePane(
                        message = state.error.orEmpty(),
                        actionText = stringResource(R.string.retry),
                        onAction = refresh,
                    )
                }

                state.problems.isEmpty() -> {
                    MessagePane(
                        message = stringResource(R.string.all_good),
                        actionText = stringResource(R.string.update),
                        onAction = refresh,
                    )
                }

                else -> {
                    Column(Modifier.fillMaxSize()) {
                        state.error?.let { ErrorBanner(it) }
                        LazyColumn(Modifier.fillMaxSize()) {
                            items(state.problems, key = { it.eventid }) { problem ->
                                ProblemRow(problem, onClick = { selected = problem })
                            }
                        }
                    }
                }
            }
        }
    }

    selected?.let { problem ->
        ProblemDetailsDialog(problem = problem, onDismiss = { selected = null })
    }
}

/**
 * Счётчик в шапке: «показано / всего», без слов. Плюс означает, что проблем
 * на сервере больше, чем мы просматриваем за один опрос.
 */
private fun problemsTitle(state: ProblemListState): String {
    val page = state.page ?: return if (state.loading) "…" else "—"
    val plus = if (page.totalKnown) "" else "+"
    return "${page.shown.size}/${page.total}$plus"
}

@Composable
private fun ProblemRow(problem: Problem, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(IntrinsicSize.Min)
            .clickable(onClick = onClick),
    ) {
        Box(
            Modifier
                .width(6.dp)
                .fillMaxHeight()
                .background(problemColor(problem)),
        )
        Column(
            modifier = Modifier
                .weight(1f)
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = ZabbixTime.format(problem.clock),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (problem.resolved) {
                    Text(
                        text = " · " + stringResource(R.string.resolved),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(
                text = problem.hostName,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = problem.name,
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
    HorizontalDivider()
}

/** Цвет полосы важности; у восстановленной проблемы он приглушён. */
@Composable
private fun problemColor(problem: Problem): Color {
    if (problem.resolved) return MaterialTheme.colorScheme.outlineVariant
    return when (problem.severityLevel) {
        5 -> Color(0xFFDC0000) // Disaster
        4 -> Color(0xFFFF9900) // High
        3 -> Color(0xFFFFFF00) // Average
        2 -> Color(0xFFFFC864) // Warning
        1 -> Color(0xFF6496FF) // Information
        else -> MaterialTheme.colorScheme.outline
    }
}

@Composable
private fun MessagePane(message: String, actionText: String, onAction: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(text = message, textAlign = TextAlign.Center)
        Button(onClick = onAction, modifier = Modifier.padding(top = 16.dp)) {
            Text(actionText)
        }
    }
}

@Composable
private fun ErrorBanner(message: String) {
    Text(
        text = message,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.error,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
    )
    HorizontalDivider()
}

@Composable
private fun ProblemDetailsDialog(problem: Problem, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.close)) }
        },
        title = { Text(stringResource(R.string.problem_details)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                DetailLine(stringResource(R.string.time), ZabbixTime.format(problem.clock))
                DetailLine(stringResource(R.string.host), problem.hostName)
                DetailLine(stringResource(R.string.problem), problem.name)
                DetailLine(stringResource(R.string.severity), severityName(problem.severityLevel))
                DetailLine(
                    stringResource(R.string.status),
                    stringResource(
                        if (problem.resolved) R.string.status_resolved else R.string.status_active,
                    ),
                )
                if (problem.resolved) {
                    DetailLine(stringResource(R.string.recovery_time), ZabbixTime.format(problem.recoveryClock))
                }
                DetailLine(
                    stringResource(R.string.acknowledged),
                    stringResource(if (problem.acknowledged == "1") R.string.yes else R.string.no),
                )
            }
        },
    )
}

@Composable
private fun DetailLine(label: String, value: String) {
    Column {
        Text(
            text = "$label:",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(text = value, style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun severityName(level: Int): String = stringResource(
    when (level) {
        5 -> R.string.severity_5
        4 -> R.string.severity_4
        3 -> R.string.severity_3
        2 -> R.string.severity_2
        1 -> R.string.severity_1
        0 -> R.string.severity_0
        else -> R.string.severity_unknown
    },
)
