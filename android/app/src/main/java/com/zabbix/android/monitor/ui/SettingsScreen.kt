package com.zabbix.android.monitor.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.zabbix.android.monitor.R
import com.zabbix.android.monitor.data.MonitorSettings
import com.zabbix.android.monitor.data.ZabbixApi
import com.zabbix.android.monitor.data.ZabbixUrl
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Настройки. Действия — значками в шапке: закрыть, проверить подключение,
 * сохранить. Текстом внизу остаётся только результат проверки.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    initial: MonitorSettings,
    canCancel: Boolean,
    onCancel: () -> Unit,
    onSaved: (MonitorSettings) -> Unit,
) {
    var url by rememberSaveable { mutableStateOf(initial.url) }
    var token by rememberSaveable { mutableStateOf(initial.token) }
    var insecureTls by rememberSaveable { mutableStateOf(initial.allowInsecureTls) }
    var limitText by rememberSaveable { mutableStateOf(initial.safeLimit().toString()) }
    var intervalText by rememberSaveable {
        mutableStateOf(initial.safeIntervalSeconds().toString())
    }

    var testing by remember { mutableStateOf(false) }
    /** null — ещё не проверяли, true/false — результат последней проверки. */
    var testOk by remember { mutableStateOf<Boolean?>(null) }
    var testError by remember { mutableStateOf("") }

    val scope = rememberCoroutineScope()

    val limit = limitText.toIntOrNull()
    val limitValid = limit != null && limit in 1..ZabbixApi.MAX_LIMIT
    val interval = intervalText.toIntOrNull()
    val intervalValid = interval != null &&
        interval in MonitorSettings.MIN_INTERVAL_SECONDS..MonitorSettings.MAX_INTERVAL_SECONDS

    val canSave = url.isNotBlank() && token.isNotBlank() && limitValid && intervalValid

    // initial.copy(...): поля, которых нет в этом списке, не теряются
    val save: () -> Unit = {
        onSaved(
            initial.copy(
                url = ZabbixUrl.normalize(url),
                token = token.trim(),
                allowInsecureTls = insecureTls,
                limit = limit ?: initial.limit,
                intervalSeconds = interval ?: initial.intervalSeconds,
            ),
        )
    }

    val runTest: () -> Unit = {
        testing = true
        testOk = null
        scope.launch {
            try {
                val api = ZabbixApi(url, token.trim(), insecureTls)
                withContext(Dispatchers.IO) { api.problems(maxFetch = 1) }
                testOk = true
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                testOk = false
                testError = e.message ?: e.javaClass.simpleName
            } finally {
                testing = false
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings)) },
                navigationIcon = {
                    if (canCancel) {
                        IconButton(onClick = onCancel) {
                            Icon(
                                imageVector = Icons.Filled.Close,
                                contentDescription = stringResource(R.string.cancel),
                            )
                        }
                    }
                },
                actions = {
                    if (testing) {
                        Box(
                            modifier = Modifier.size(48.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(20.dp),
                                strokeWidth = 2.dp,
                            )
                        }
                    } else {
                        IconButton(onClick = runTest, enabled = canSave) {
                            Icon(
                                imageVector = Icons.Filled.PlayArrow,
                                contentDescription = stringResource(R.string.test_connection),
                            )
                        }
                    }
                    IconButton(onClick = save, enabled = canSave) {
                        Icon(
                            imageVector = Icons.Filled.Check,
                            contentDescription = stringResource(R.string.save),
                        )
                    }
                },
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .padding(innerPadding)
                .padding(horizontal = 16.dp, vertical = 12.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            OutlinedTextField(
                value = url,
                onValueChange = { url = it },
                label = { Text(stringResource(R.string.url_server)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = token,
                onValueChange = { token = it },
                label = { Text(stringResource(R.string.token)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth(),
            )

            OutlinedTextField(
                value = limitText,
                onValueChange = { input -> limitText = input.filter { it.isDigit() }.take(5) },
                label = { Text(stringResource(R.string.problem_limit)) },
                singleLine = true,
                isError = !limitValid,
                supportingText = {
                    Text(
                        if (limitValid) {
                            stringResource(R.string.problem_limit_hint)
                        } else {
                            stringResource(R.string.error_invalid_number)
                        },
                    )
                },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth(),
            )

            OutlinedTextField(
                value = intervalText,
                onValueChange = { input -> intervalText = input.filter { it.isDigit() }.take(4) },
                label = { Text(stringResource(R.string.refresh_interval)) },
                singleLine = true,
                isError = !intervalValid,
                supportingText = {
                    Text(
                        if (intervalValid) {
                            stringResource(R.string.refresh_interval_hint)
                        } else {
                            stringResource(R.string.error_invalid_number)
                        },
                    )
                },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth(),
            )

            Row(verticalAlignment = Alignment.CenterVertically) {
                Switch(checked = insecureTls, onCheckedChange = { insecureTls = it })
                Text(
                    text = stringResource(R.string.self_signed),
                    modifier = Modifier.padding(start = 12.dp),
                )
            }

            if (ZabbixUrl.isCleartext(url)) {
                Text(
                    text = stringResource(R.string.cleartext_warning),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }

            when (testOk) {
                true -> Text(
                    text = stringResource(R.string.connection_ok),
                    color = MaterialTheme.colorScheme.primary,
                    style = MaterialTheme.typography.bodyMedium,
                )

                false -> Text(
                    text = stringResource(R.string.connection_failed, testError),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )

                null -> Unit
            }
        }
    }
}
