import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    // Spike-тест (різкий стрибок)
    stages: [
        { duration: '10s', target: 200 }, // За 10 секунд різко піднімаємо до 50 юзерів
        { duration: '40s', target: 200 }, // Тримаємо цей жорсткий пік півхвилини
        { duration: '10s', target: 0 },  // Швидко згортаємо
    ],
    thresholds: {
        // При такому навантаженні ми вже не вимагаємо швидкості, 
        // головне — перевірити, чи сервер не почне "відкидати" запити з помилками
        http_req_failed: ['rate<0.05'], // Допускаємо до 5% помилок (таймаутів)
    },
};

const BASE_URL = 'https://138.68.69.86.nip.io/api';
const VERIFIED_EMAIL = 'ivan@example.com';
const USER_PASSWORD = 'Secret123!';

export default function () {
    const params = {
        headers: { 'Content-Type': 'application/json' },
    };

    // === КРОК 1: Важке навантаження на CPU (Логін) ===
    const loginPayload = JSON.stringify({
        email: VERIFIED_EMAIL,
        password: USER_PASSWORD,
    });

    const loginRes = http.post(`${BASE_URL}/auth/login`, loginPayload, params);
    
    const isLoginSuccessful = check(loginRes, {
        'login status is 200': (r) => r.status === 200,
    });

    // === КРОК 2: Навантаження на БД (Паралельні запити) ===
    if (isLoginSuccessful) {
        const token = loginRes.json('access_token');

        const authParams = {
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`, 
            },
        };

        // Запускаємо два запити ОДНОЧАСНО за допомогою http.batch
        // Це імітує реалістичне завантаження сторінки "Мій кабінет" на фронтенді
        const responses = http.batch([
            ['GET', `${BASE_URL}/users/me`, null, authParams],
            ['GET', `${BASE_URL}/users/me/addresses`, null, authParams],
        ]);

        check(responses[0], {
            'get profile is 200': (r) => r.status === 200,
        });
        check(responses[1], {
            'get addresses is 200': (r) => r.status === 200,
        });
    }

    sleep(1); // Секунда перепочинку між ітераціями
}