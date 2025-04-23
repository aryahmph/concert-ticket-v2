import {SharedArray} from "k6/data";
import http from "k6/http";
import {check, fail} from "k6";
import {Counter} from 'k6/metrics';

const BaseUrl = 'http://localhost:8080/api';

const CounterTicketRush = new Counter('ticket_rush');
const CounterSameEmail = new Counter('same_email');
const CounterOrder = new Counter('order');

export const options = {
    summaryTrendStats: ['avg', 'p(90)'], scenarios: {
        concertTicket: {
            executor: 'constant-vus', vus: 2, duration: '2m',
        }
    }, thresholds: {
        'http_req_duration{ListCategories:get}': [],
        'http_req_duration{CreateOrder:post}': [],
        'http_req_duration{PaymentCallback:post}': [],
    },
}

http.setResponseCallback(http.expectedStatuses(200, 409));

const data = new SharedArray('some name', function () {
    return JSON.parse(open('./emails.json')).emails;
});

const emailLength = data.length;

export default function () {
    const categoriesRes = http.get(`${BaseUrl}/categories`, {
        tags: {ListCategories: 'get'},
    });

    if (!check(categoriesRes, {'is status OK': (r) => r.status === 200})) {
        console.log(`List tickets failed: ${categoriesRes.body}`);
        fail('Failed to list tickets');
    }

    const categories = categoriesRes.json();
    let randomCategory;
    for (let i = 0; i < 5; i++) {
        randomCategory = categories[Math.floor(Math.random() * categories.length)];
        if (randomCategory.quantity > 0) {
            break;
        }
    }

    const createOrderReq = {
        'name': 'Dummy',
        phone: '+6281234567890',
        'email': data[Math.floor(Math.random() * emailLength)],
        'category_id': randomCategory.id,
    };

    const orderRes = http.post(`${BaseUrl}/orders`, JSON.stringify(createOrderReq), {
        headers: {'Content-Type': 'application/json'}, tags: {CreateOrder: 'post'},
    });

    check(orderRes, {'is status OK': (r) => r.status === 200 || r.status === 409});
    if (orderRes.status === 500) {
        console.log(`Create order failed: ${orderRes.body}`)
        fail('Failed to create order');
    }

    if (orderRes.status === 409) {
        const errorMessages = orderRes.json().error
        if (errorMessages === 'category quantity is exhausted') {
            CounterTicketRush.add(1);
        } else if (errorMessages === 'order already exist') {
            CounterSameEmail.add(1);
        }
        return;
    }

    CounterOrder.add(1);

    if (__VU % 5 === 0) {
        return;
    }

    console.log(`Order created: ${orderRes.body}`);
    const externalId = orderRes.json().external_id;
    payOrderAsync({'external_id': externalId});
}

function payOrderAsync(payOrderReq) {
    const delay = Math.floor(Math.random() * 3000) + 1000;

    setTimeout(() => {
        const payOrderRes = http.post(`${BaseUrl}/payments/callback`, JSON.stringify(payOrderReq), {
            headers: {'Content-Type': 'application/json'}, tags: {PaymentCallback: 'post'},
        });

        if (!check(payOrderRes, {'is status OK': (r) => r.status === 200})) {
            console.log(`Pay order failed: ${payOrderRes.body}`);
            fail('Failed to pay order');
        }
    }, delay);
}