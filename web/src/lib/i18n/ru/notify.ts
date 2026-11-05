// The Russian words of the alerts that reach the person: on the page while it is in front, and the alerts panel's
// word on this device — its browser and system, permission, subscription, the last push sent and received.

import type { Dictionary } from "../../i18n";

export const notify: Dictionary = {
  "Alerts show on this page while it is in front.": "Уведомления показываются на этой странице, пока она на переднем плане.",
  "Firefox blocks notifications for this site: click the icon left of the address, then Permissions → Send notifications → Allow, and reload.":
    "Firefox блокирует уведомления этого сайта: нажми значок слева от адреса, затем «Разрешения» → «Отправлять уведомления» → «Разрешить», и обнови страницу.",
  "Safari blocks notifications for this site: Safari → Settings → Websites → Notifications → this site → Allow, and reload.":
    "Safari блокирует уведомления этого сайта: Safari → Настройки → Веб-сайты → Уведомления → этот сайт → «Разрешить», и обнови страницу.",
  "{browser} blocks notifications for this site: click the icon left of the address → Site settings → Notifications → Allow, and reload.":
    "{browser} блокирует уведомления этого сайта: нажми значок слева от адреса → «Настройки сайтов» → «Уведомления» → «Разрешить», и обнови страницу.",
  "The browser blocks notifications for this site: allow them in its site settings, and reload.": "Браузер блокирует уведомления этого сайта: разреши их в настройках сайта и обнови страницу.",
  "the browser": "браузер",
  "Nothing appears? System Settings → Notifications → {browser}: Allow notifications, as Banners or Alerts — and Focus off.":
    "Ничего не появляется? Системные настройки → Уведомления → {browser}: «Разрешить уведомления», стиль «Баннеры» или «Предупреждения» — и режим «Фокусирование» выключен.",
  "Nothing appears? Settings → System → Notifications: {browser} on — and Do not disturb off.": "Ничего не появляется? Параметры → Система → Уведомления: {browser} включён — и «Не беспокоить» выключено.",
  "Nothing appears? Your desktop's notification settings must let {browser} show them, with Do not disturb off.": "Ничего не появляется? В настройках уведомлений рабочего стола {browser} должен быть разрешён, а «Не беспокоить» выключено.",
  "On iPhone and iPad notifications come only to remuda added to the Home Screen; Settings → Notifications → remuda lets them through.":
    "На iPhone и iPad уведомления приходят только в remuda, добавленный на экран «Домой»; Настройки → Уведомления → remuda их разрешает.",
  "Nothing appears? Settings → Apps → {browser} → Notifications must be on.": "Ничего не появляется? Настройки → Приложения → {browser} → Уведомления должны быть включены.",
  "Nothing appears? The system's notification settings must let {browser} show them.": "Ничего не появляется? Системные настройки уведомлений должны разрешать {browser} их показывать.",
  "No push sent to this browser since remuda started.": "С запуска remuda этому браузеру ничего не отправлялось.",
  "Last push sent {when}: {title} — refused: {error}": "Последнее отправлено {when}: {title} — отклонено: {error}",
  "Last push sent {when}: {title} — the push service took it.": "Последнее отправлено {when}: {title} — служба push его приняла.",
  "No push received here yet.": "Сюда ещё ничего не приходило.",
  "Last push received here {when}: {title} — shown on the page, which had the focus.": "Последнее получено здесь {when}: {title} — показано на странице, она была в фокусе.",
  "Last push received here {when}: {title} — handed to the system to show.": "Последнее получено здесь {when}: {title} — передано системе для показа.",
  "This browser is not subscribed to this cockpit: the switches below shape only what shows on this page. Turn on for notifications with the page closed.":
    "Этот браузер не подписан на эту кабину: переключатели ниже влияют только на то, что показывается на странице. Включи, чтобы получать уведомления и при закрытой странице.",
  "This browser holds a subscription this cockpit does not know — one made for another address. Turn off and on again.":
    "У браузера есть подписка, которую эта кабина не знает — она сделана для другого адреса. Выключи и включи снова.",
  "This browser is subscribed to this cockpit.": "Этот браузер подписан на эту кабину.",
  "{browser} on {system}": "{browser} на {system}",
  "This browser": "Этот браузер",
  "this system": "этой системе",
  "notifications allowed": "уведомления разрешены",
  "notifications blocked": "уведомления заблокированы",
  "not asked yet": "ещё не спрашивали",
  "no notifications here": "уведомлений здесь нет",
  "While this page is in front, every alert below shows in its corner as well.": "Пока эта страница на переднем плане, каждое уведомление ниже показывается и в её углу.",
  "A chime": "Звук",
  "with an alert shown on the page": "при уведомлении на странице",
  "Every finish 10 s after it; or only turns that worked a minute or more, a minute after.": "Каждое завершение — через 10 с; или только ходы дольше минуты — через минуту.",
  "every finish": "каждое завершение",
  "turns over a minute": "ходы дольше минуты",
  "A test is up in the corner of this page. Turn on push for a system notification too.": "Тест показан в углу этой страницы. Включи push, чтобы пришло и системное уведомление.",
  "A test is up in the corner of this page, and a system notification should appear in a moment.": "Тест показан в углу этой страницы, а системное уведомление должно появиться через мгновение.",
  "Send a test": "Отправить тест",
  "The browser's push service gave no answer in 20 s.": "Служба push браузера не ответила за 20 с.",
  "The browser would not register for push ({reason}). A private window, a headless browser or one without a push service cannot get pushes; the alerts still show on this page while it is in front.":
    "Браузер не зарегистрировался для push ({reason}). В приватном окне, в браузере без интерфейса или без службы push уведомления не приходят; на этой странице они всё равно показываются, пока она на переднем плане.",
};
