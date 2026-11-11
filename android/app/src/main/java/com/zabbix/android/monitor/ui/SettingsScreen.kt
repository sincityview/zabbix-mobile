package com.zabbix.android.monitor.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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

    var testing by remember { mutableStateOf(false) }
    /** null — ещё не проверяли, true/false — результат последней проверки. */
    var testOk by remember { mutableStateOf<Boolean?>(null) }
    var testError by remember { mutableStateOf("") }

    val scope = rememberCoroutineScope()
    val canSave = url.isNotBlank() && token.isNotBlank()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings)) },
                navigationIcon = {
                    if (canCancel) {
                        TextButton(onClick = onCancel) { Text(stringResource(R.string.cancel)) }
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

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    enabled = canSave && !testing,
                    onClick = {
                        testing = true
                        testOk = null
                        scope.launch {
                            try {
                                val api = ZabbixApi(url, token.trim(), insecureTls)
                                withContext(Dispatchers.IO) { api.problems(limit = 1) }
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
                    },
                ) {
                    Text(stringResource(if (testing) R.string.checking else R.string.test_connection))
                }

                Button(
                    enabled = canSave,
                    onClick = {
                        onSaved(
                            MonitorSettings(
                                url = ZabbixUrl.normalize(url),
                                token = token.trim(),
                                allowInsecureTls = insecureTls,
                            ),
                        )
                    },
                ) {
                    Text(stringResource(R.string.save))
                }
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
