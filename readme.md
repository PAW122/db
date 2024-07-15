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


# bentchmark

* tests were conducted for comparative purposes and may not be reliable

* tests were performed using the default configuration

- bson :

    + V 1.0
    save: +/- 1:56:00   (mm:ss:ms)
    read: +/- 00:06:00

    + V 1.1
    save: +/- 00:03:73
    read: +/- 00:03:38

- json :
    + V 1.0
    save: N/A
    read: N/A

    + V 1.1
    save: +/- 00:03:69
    read: +/- 00:03:32


Test code:

```js

const axios = require('axios');

 const serverUrl = 'http://localhost:5432'
 const api_key = 'database_acces_api_key'
// Generate sample data
function generateSampleData(index) {
    return {
        user: {
            age: 20 + index,
            name: `User_${index}`
        },
        some_float: 1.5 + index,
        key: `${index}`
    };
}

// Function to save data to the server
async function saveData(key, data) {
    try {
        const response = await axios.post(`${serverUrl}/save?path=${key}`, data,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        return response.data;
    } catch (error) {
        throw new Error(`Error saving data: ${error.message}`);
    }
}

// Function to read data from the server
async function readData(key) {
    try {
        const response = await axios.get(`${serverUrl}/read?path=${key}`,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        return response.data;
    } catch (error) {
        throw new Error(`Error reading data: ${error.message}`);
    }
}

// Function to delete data from the server
async function deleteData(key) {
    try {
        const response = await axios.get(`${serverUrl}/delete?path=${key}`,
            {
                headers: {
                    'X-API-Key': api_key
                }
            });
        return response.data;
    } catch (error) {
        throw new Error(`Error deleting data: ${error.message}`);
    }
}

// Function to test database operations
async function testDatabaseOperations() {
    const numEntries = 10000;
    const sampleData = Array.from({ length: numEntries }, (_, index) => generateSampleData(index));

    // Save operation
    console.time('Save Time');
    for (let i = 0; i < numEntries; i++) {
        await saveData(sampleData[i].key, sampleData[i]);
    }
    console.timeEnd('Save Time');

    // Read operation
    console.time('Read Time');
    for (let i = 0; i < numEntries; i++) {
        const key = `${i}`; // adjust key if necessary based on your server implementation
        const data = await readData(key);
        // console.log(data)
        // Optionally, verify data here
    }
    console.timeEnd('Read Time');

    // Delete operation (assuming you're deleting 'some_float' field)
    // console.time('Delete Time');
    // for (let i = 0; i < numEntries; i++) {
    //     await deleteData('some_float');
    // }
    // console.timeEnd('Delete Time');
}

// Run the test
testDatabaseOperations();

```