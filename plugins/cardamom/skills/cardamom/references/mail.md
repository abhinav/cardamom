# Draw attention to work

Use Cardamom mail when the collaboration runtime does not already provide
the needed notification channel.
Mail is store-scoped and expires; keep lasting decisions and outcomes in issues.
A message does not assign a claim or change readiness.

```bash
card --actor <sender> mail send <recipient> 'Issue cm-abcd is ready to continue.'
card --actor <recipient> mail recv
```

For topic subscriptions, use `mail subscribe` and `mail publish` help.
Renew subscriptions separately; `mail recv --tail` only receives.
Subscriptions and messages expire independently.
Select the same store for sender and recipient; separate stores do not exchange mail.
