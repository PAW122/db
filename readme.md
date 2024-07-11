## Pawiu DB

# example usage

```js
const axios = require("axios");
const serverUrl = "http://localhost:5432";
const api_key = "database_acces_api_key";

const dataToSave = {
    user: {
        age: 15,
        name: "John"
    },
    some_float: 1.5
};

async function saveData() {
    try {
        const response = await axios.post(`${serverUrl}/save?path=`, dataToSave,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        console.log('Data saved successfully:', response.data);
    } catch (error) {
        console.error('Error saving data:', error.message);
    }
}

async function add() {
    try {
        const response = await axios.post(`${serverUrl}/add?path=user`, { name: "test" },
            {
                headers: {
                    'X-API-Key': api_key
                }
            });

        console.log('Data added":', response.data);
    } catch (error) {
        console.error('Error saving data under "user":', error.message);
    }
}

async function delete() {
    try {
        const response = await axios.get(`${serverUrl}/delete?path=some_float`,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        console.log('Data deleted":', response.data);
    } catch (error) {
        console.error('Error saving data under "user":', error.message);
    }
}

async function readData(key) {
    try {
        const response = await axios.get(`${serverUrl}/read?path=${key}`,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        console.log('Data read successfully:', response.data);
    } catch (error) {
        console.error('Error reading data:', error.message);
    }
}

```
