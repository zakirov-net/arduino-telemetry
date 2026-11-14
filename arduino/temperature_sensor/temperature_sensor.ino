// LM35 на A0, LED на D7. Протокол: строка команды + '\n', ответ — JSON + '\n'.
//   GET\n      -> {"temperature":23.50,"led":false}\n
//   PING\n     -> {"pong":true}\n
//   LED:ON\n   -> {"ok":true,"led":true}\n
//   LED:OFF\n  -> {"ok":true,"led":false}\n
//   иное       -> {"error":"unknown_command","cmd":"..."}\n

const int   sensorPin    = A0;
const int   ledPin       = 7;
const int   numReadings  = 10;
const unsigned long adcRefStabilizeMs = 10;

const float vRefMv        = 1000.0f; // измеренное опорное
const float lm35MvPerDegC = 10.0f;

bool ledState = false;

void setup() {
  Serial.begin(9600);
  Serial.setTimeout(50);

  pinMode(ledPin, OUTPUT);
  digitalWrite(ledPin, LOW);           // при старте LED выключен

  analogReference(INTERNAL);
  delay(adcRefStabilizeMs);
  (void)analogRead(sensorPin);
}

float readTemperatureC() {
  long sum = 0;
  for (int i = 0; i < numReadings; i++) {
    sum += analogRead(sensorPin);
    delay(10);
  }
  float avgRaw    = (float)sum / numReadings;
  float voltageMv = (avgRaw / 1024.0f) * vRefMv;
  return voltageMv / lm35MvPerDegC;
}

void sendUnknown(const String& cmd) {
  String safe = cmd;
  safe.replace("\"", "'");
  Serial.print("{\"error\":\"unknown_command\",\"cmd\":\"");
  Serial.print(safe);
  Serial.println("\"}");
}

void loop() {
  if (Serial.available() <= 0) return;

  String cmd = Serial.readStringUntil('\n');
  cmd.trim();
  if (cmd.length() == 0) return;

  if (cmd == "GET") {
    float t = readTemperatureC();
    Serial.print("{\"temperature\":");
    Serial.print(t, 2);
    Serial.print(",\"led\":");
    Serial.print(ledState ? "true" : "false");
    Serial.println("}");
  } else if (cmd == "PING") {
    Serial.println("{\"pong\":true}");
  } else if (cmd == "LED:ON") {
    digitalWrite(ledPin, HIGH);
    ledState = true;
    Serial.println("{\"ok\":true,\"led\":true}");
  } else if (cmd == "LED:OFF") {
    digitalWrite(ledPin, LOW);
    ledState = false;
    Serial.println("{\"ok\":true,\"led\":false}");
  } else {
    sendUnknown(cmd);
  }
}
